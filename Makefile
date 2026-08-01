IMAGE_NAME := blockchain-insight
ENV_FILE := $(PWD)/cmd/app.env

docker-build:
	docker build -t $(IMAGE_NAME):latest .

docker-run:
	docker run -p 5050:5050 -v $(ENV_FILE):/app/app.env $(IMAGE_NAME):latest

docker-stop:
	docker ps -q --filter ancestor=$(IMAGE_NAME) | xargs -r docker stop	