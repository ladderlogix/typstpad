#set page(paper: "a4", margin: (x: 1.8cm, y: 1.6cm))
#set text(size: 10.5pt)
#set par(justify: true)

#let section(title) = {
  v(0.4cm)
  text(size: 13pt, weight: "bold")[#title]
  line(length: 100%, stroke: 0.5pt)
  v(0.15cm)
}

#let entry(role, org, dates, ..details) = {
  grid(
    columns: (1fr, auto),
    [*#role*, #org], align(right)[#dates],
  )
  for d in details.pos() [ - #d ]
  v(0.2cm)
}

#align(center)[
  #text(size: 22pt, weight: "bold")[Jane Doe]
  #v(0.1cm)
  jane\@example.com · +1 555 0100 · City, Country · github.com/janedoe
]

#section[Experience]
#entry("Senior Engineer", "Acme Corp", "2022 — present",
  [Led the migration of the billing platform to an event-driven architecture.],
  [Mentored four engineers; introduced code-review and testing practices.],
)
#entry("Software Engineer", "Widgets Inc", "2019 — 2022",
  [Built and operated customer-facing APIs serving 40M requests/day.],
)

#section[Education]
#entry("M.Sc. Computer Science", "Sample University", "2017 — 2019",
  [Thesis: “Interesting Things About Distributed Systems”.],
)

#section[Skills]
Languages: Go, TypeScript, Python, SQL \
Infrastructure: Kubernetes, Postgres, Terraform, observability tooling \
Interests: typesetting beautiful documents with Typst
