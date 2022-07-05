MAIN_APP_PATH = ./src
GO = go
ENV = GOOS=linux GOARCH=amd64
LD_FLAGS = -s -w
OUT = ./eve-industry

run:
	$(GO) run $(MAIN_APP_PATH) $(ARGS)

build:
	@echo "Building willied..."
	$(ENV) $(GO) build -o $(OUT) -ldflags="$(LD_FLAGS)" $(MAIN_APP_PATH)

deploy:
	@echo "Deploying willied..."
	@echo "TODO"

clean:
	rm -rfv $(OUT)
