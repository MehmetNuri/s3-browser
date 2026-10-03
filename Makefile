.PHONY: install build dev test frontend linux deb rpm archpkg appimage flatpak windows dmg
install:
	npm ci
	npm --prefix frontend ci
build:
	npm run build
dev:
	npm run dev
test:
	cd backend && go test -race ./...
	npm run check
frontend:
	npm run build:frontend
linux:
	npm run package:linux
deb rpm archpkg appimage flatpak: build
	npx electron-builder --linux $(if $(filter archpkg,$@),pacman,$(if $(filter appimage,$@),AppImage,$@)) --x64 --publish never
windows:
	npm run package:windows
dmg:
	npm run package:mac
