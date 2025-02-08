build:
	go build -o out/swissknife cmd/paged/main.go

build.onepage:
	go build -o out/swissknifeone cmd/single/main.go

run: build
	./out/swissknife -cfg=./commands.1.yaml,./commands.2.yaml

run.onepage:
	./out/swissknife -cfg=./commands.yaml

build.linux:
	docker build --platform linux/amd64 -t swissknife-builder-compressed .
	docker run --name swissknife-linux-builder swissknife-builder-compressed
	docker cp swissknife-linux-builder:/output/swissknife ./out/linux/
	docker rm swissknife-linux-builder
