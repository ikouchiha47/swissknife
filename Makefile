build:
	go build -o out/swissknife cmd/paged/main.go

build.onepage:
	go build -o out/swissknifeone cmd/single/main.go

run: build
	./swissknife -cfg=./commands.1.yaml,./commands.1.yaml

run.onepage:
	./swissknife -cfg=./commands.yaml
