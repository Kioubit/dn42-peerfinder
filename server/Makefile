BINARY := peerfinder
BUILDFLAGS=-trimpath -tags sqlite_omit_load_extension

build:
	go build -o bin/${BINARY} .

release:
	go build -ldflags="-s -w -extldflags=-static" ${BUILDFLAGS} -o bin/${BINARY} .

clean:
	if [ -d "bin/" ]; then find bin/ -type f -delete ;fi
	if [ -d "bin/" ]; then rm -d bin/ ;fi
