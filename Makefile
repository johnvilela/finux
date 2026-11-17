.PHONY: dev prod

dev:
	go build -gcflags="all=-N -l" -o bin/finux-dev ./src

prod:
	go build -trimpath -ldflags="-s -w" -o bin/finux ./src
