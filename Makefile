.PHONY: web build run collab dev deploy

web:
	cd web && npm run build

build: web
	go build -o typstpad ./cmd/typstpad

collab:
	cd collab && npm run dev

run: build
	./typstpad serve

up:
	docker compose up -d --build

deploy:
	rsync -az --delete --exclude node_modules --exclude .git --exclude web/dist --exclude collab/dist \
		--exclude /typstpad --exclude data --exclude .env ./ hwsec-strategist:~/typstpad/
	ssh hwsec-strategist 'cd ~/typstpad && docker compose up -d --build'
