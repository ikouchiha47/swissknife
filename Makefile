run:
	go build -o out/swissknife main.go
	./swissknife -cfg=./commands.yaml
