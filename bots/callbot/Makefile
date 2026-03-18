CGO_CFLAGS  := -I/opt/homebrew/include
CGO_LDFLAGS := -L/opt/homebrew/lib

.PHONY: run build

run:
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go run .

build:
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go build -o callbot .
