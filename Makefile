# DN42 Peer Finder
#
# Targets:
#   frontend  - Build the Vite frontend (npm run build)
#   www       - Copy frontend/dist/ + agent files into server/www/
#   server    - Build the Go server (includes data-assets generation)
#   data-dir  - Prepare data directory for running locally
#   build     - Default target for running locally & debug
#   release   - Build a static release binary via server/Makefile
#   clean     - Remove all build artifacts
#   clean-all - Remove all build artifacts and generated files

.PHONY: frontend www server data-dir build release clean clean-all

build: server data-dir

data-dir:
	mkdir -p data/measurements/
	cd data && ./archive.sh

frontend:
	cd frontend && npm run build

www: frontend
	mkdir -p server/www/agent
	cp -r frontend/dist/* server/www/
	cp agent/peerfinder-agent.service server/www/agent/
	cp agent/peerfinder-agent.py server/www/agent/
	cp agent/install.sh server/www/agent/

server: www
	$(MAKE) -C server build

release: www
	$(MAKE) -C server release

clean:
	$(MAKE) -C server clean
	rm -rf server/www/
	rm -rf frontend/dist/
	rm -rf data/access.lock
	rm -rf data/archive.zip

clean-all: clean
	$(MAKE) -C server clean-all
	rm -rf data/measurements/
