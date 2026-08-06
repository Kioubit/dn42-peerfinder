# peerfinder.dn42.dev - DN42 Peer Finder
#
# Targets:
#   frontend - Build the Vite frontend (npm run build)
#   www      - Copy frontend/dist/ + agent files into server/www/
#   server   - Build the Go server (includes data-assets generation)
#   build    - Default target
#   release  - Build a static release binary via server/Makefile
#   clean    - Remove all build artifacts

.PHONY: frontend www server build release clean

build: server

frontend:
	cd frontend && npm run build

www: frontend
	mkdir -p server/www/agent
	cp -r frontend/dist/* server/www/
	cp agent/peerfinder-agent.service server/www/agent/
	cp agent/peerfinder-agent.py server/www/agent/

server: www
	$(MAKE) -C server build

release: www
	$(MAKE) -C server release

clean:
	$(MAKE) -C server clean
	rm -rf server/www/
	rm -rf frontend/dist/
