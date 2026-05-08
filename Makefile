all:
	go build ./cmd/moor

windows:
	GOOS=windows go build ./cmd/moor
