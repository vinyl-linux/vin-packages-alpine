default: loader

loader: bin/*.go bin/go.mod bin/go.sum
	(cd bin && CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o ../$@)
