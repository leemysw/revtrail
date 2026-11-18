APP_PLATFORM := darwin/$(shell go env GOHOSTARCH)
APP_RUN_OUT := $(abspath build/app-run)
APP_BUNDLE := $(APP_RUN_OUT)/$(subst /,-,$(APP_PLATFORM))/Revtrail.app

.PHONY: app-run

app-run:
	@osascript -l JavaScript scripts/quit-running-app.js "$$(plutil -extract identifier raw mygo.json)"
	go tool mygo build -platform $(APP_PLATFORM) -skip-dmg -o "$(APP_RUN_OUT)"
	"$(APP_BUNDLE)/Contents/MacOS/Revtrail"
