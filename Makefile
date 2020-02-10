gogit:
	go build -o bin/gogit -ldflags '-extldflags "-static"' github.com/husio/gogit

.PHONY: gogit
