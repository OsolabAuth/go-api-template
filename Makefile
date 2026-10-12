.PHONY: db-up dev migrate-up migrate-down test coverage build deploy

db-up:
	docker compose up -d db

dev:
	@set -a; if [ -f .env ]; then . ./.env; fi; set +a; exec go run ./cmd/api

migrate-up:
	docker compose --profile tools run --rm migrate up

migrate-down:
	docker compose --profile tools run --rm migrate down 1

test:
	go test ./...

coverage:
	mkdir -p coverage
	rm -f coverage/coverage.out coverage/summary.txt coverage/index.html
	go test -count=1 -coverpkg=./... -covermode=atomic -coverprofile=coverage/coverage.out ./...
	go tool cover -func=coverage/coverage.out > coverage/summary.txt
	cat coverage/summary.txt
	go tool cover -html=coverage/coverage.out -o coverage/index.html

build:
	docker build -f docker/Dockerfile.prod -t go-api-template:local .

# Requires GCP_PROJECT, GCP_REGION, AR_REPOSITORY, SERVICE_NAME,
# DATABASE_URL_SECRET (existing Secret Manager secret).
deploy:
	@test -n "$(GCP_PROJECT)" && test -n "$(GCP_REGION)" && test -n "$(AR_REPOSITORY)" && test -n "$(SERVICE_NAME)" && test -n "$(DATABASE_URL_SECRET)" || (echo "Set GCP_PROJECT GCP_REGION AR_REPOSITORY SERVICE_NAME DATABASE_URL_SECRET"; exit 1)
	gcloud auth configure-docker $(GCP_REGION)-docker.pkg.dev --quiet
	docker build --platform linux/amd64 -f docker/Dockerfile.prod -t $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(AR_REPOSITORY)/$(SERVICE_NAME):latest .
	docker push $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(AR_REPOSITORY)/$(SERVICE_NAME):latest
	gcloud run deploy $(SERVICE_NAME) --image $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(AR_REPOSITORY)/$(SERVICE_NAME):latest --project $(GCP_PROJECT) --region $(GCP_REGION) --update-secrets DATABASE_URL=$(DATABASE_URL_SECRET):latest --update-env-vars DB_MAX_CONNS=5
