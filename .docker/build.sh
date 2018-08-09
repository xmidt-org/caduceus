GOOS=linux GOARCH=amd64 go build ../src/caduceus/

docker build -t caduceus:local .
