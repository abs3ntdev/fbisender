build: 
	go build -o dist/ .

run: build
	./dist/fbisender

tidy:
	go mod tidy

clean:
	rm -rf dist

uninstall:
	rm -f /usr/bin/fbisender

install:
	cp ./dist/fbisender /usr/bin
