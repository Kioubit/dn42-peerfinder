BINARY := peerfinder
DATA_ASSETS := directory/geomap/data-assets

.PHONY: build release clean clean-all data-assets

data-assets:
	cd $(DATA_ASSETS) && python3 download_assets.py
	cd $(DATA_ASSETS) && python3 generate_locodes.py
	cd $(DATA_ASSETS) && python3 generate_geo.py

build: data-assets
	go build -o bin/${BINARY} .

release: data-assets
	go build -ldflags="-s -w -extldflags=-static" -trimpath -tags sqlite_omit_load_extension -o bin/${BINARY} .

clean:
	rm -rf bin/
	rm -rf $(DATA_ASSETS)/generated/

clean-all: clean
	rm -rf $(DATA_ASSETS)/raw/
