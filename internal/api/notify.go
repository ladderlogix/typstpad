package api

import (
	"context"
	"log"
	"regexp"
	"strings"

	"typstpad/internal/store"
)

// mentionRe matches @handle tokens: a bare word or a full email.
var mentionRe = regexp.MustCompile(`@([A-Za-z0-9._%+\-]+(?:@[A-Za-z0-9.\-]+\.[A-Za-z]{2,})?)`)

// mentionedMembers returns the members referenced by @handle tokens in body.
// A token matches a member by full email, email local-part, or their name with
// spaces removed (all case-insensitive).
func mentionedMembers(body string, members []*store.Member) map[string]*store.Member {
	tokens := map[string]bool{}
	for _, m := range mentionRe.FindAllStringSubmatch(body, -1) {
		tokens[strings.ToLower(m[1])] = true
	}
	hit := map[string]*store.Member{}
	if len(tokens) == 0 {
		return hit
	}
	for _, mem := range members {
		email := strings.ToLower(mem.Email)
		local := email
		if i := strings.IndexByte(email, '@'); i > 0 {
			local = email[:i]
		}
		name := strings.ToLower(strings.ReplaceAll(mem.Name, " ", ""))
		if tokens[email] || tokens[local] || (name != "" && tokens[name]) {
			hit[mem.UserID] = mem
		}
	}
	return hit
}

// notifyComment fans out feed entries (and emails for mentions) when someone
// comments on a project.
func (s *Server) notifyComment(ctx context.Context, actor *store.User, p *store.Project, body string) {
	members, err := s.Store.ListMembers(ctx, p.ID)
	if err != nil {
		return
	}
	mentioned := mentionedMembers(body, members)
	link := strings.TrimRight(s.Cfg.PublicURL, "/") + "/p/" + p.ID
	preview := body
	if len(preview) > 80 {
		preview = preview[:80] + "…"
	}
	for _, mem := range members {
		if mem.UserID == actor.ID {
			continue
		}
		if _, isMention := mentioned[mem.UserID]; isMention {
			summary := actor.Name + " mentioned you in " + p.Name
			_ = s.Store.CreateNotification(ctx, mem.UserID, actor.ID, "mention", p.ID, summary)
			s.emailNotification(mem, "You were mentioned in "+p.Name, actor.Name+" mentioned you: “"+preview+"”", link)
		} else {
			summary := actor.Name + " commented on " + p.Name
			_ = s.Store.CreateNotification(ctx, mem.UserID, actor.ID, "comment", p.ID, summary)
		}
	}
}

// notifyShare records (and emails) that a project was shared with a user.
func (s *Server) notifyShare(ctx context.Context, actor *store.User, p *store.Project, target *store.User, role string) {
	if target.ID == actor.ID {
		return
	}
	summary := actor.Name + " shared “" + p.Name + "” with you (" + role + ")"
	_ = s.Store.CreateNotification(ctx, target.ID, actor.ID, "share", p.ID, summary)
	link := strings.TrimRight(s.Cfg.PublicURL, "/") + "/p/" + p.ID
	s.emailNotification(&store.Member{UserID: target.ID, Email: target.Email, Name: target.Name},
		actor.Name+" shared a project with you", summary, link)
}

func (s *Server) emailNotification(mem *store.Member, subject, line, link string) {
	if s.Mailer == nil || !s.Mailer.Enabled() || mem.Email == "" {
		return
	}
	go func() {
		if err := s.Mailer.SendNotification(mem.Email, mem.Name, subject, line, link); err != nil {
			log.Printf("notification email to %s failed: %v", mem.Email, err)
		}
	}()
}
