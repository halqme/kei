build:
  go build -o ./build/kei ./cmd/kei

test:
  go test -count=1 ./...

vet:
  go vet ./...