build::
	GOOS=linux GOARCH=amd64 go build -v -trimpath -ldflags '-s -w' -o artifacts/xlpdok-linux-amd64 ./
	GOOS=linux GOARCH=arm64 go build -v -trimpath -ldflags '-s -w' -o artifacts/xlpdok-linux-arm64 ./

push::
	docker buildx build -t cnk3x/xlpdok:dev . --platform linux/amd64,linux/arm64 --push
