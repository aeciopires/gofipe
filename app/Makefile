#---------------------------
#---------------------------
# VARIABLES
#---------------------------
#---------------------------

# Change the value according to new releases
VERSION=1.0.0

PATH_DOCKERFILE="./Dockerfile"

# Only Ubuntu
SHELL=/usr/bin/bash
# Only MacOS using brew
#SHELL=/opt/homebrew/bin/bash

APP_NAME=gofipe
BIN_FILE="bin/${APP_NAME}"

# Platforms supported. Reference: https://docs.docker.com/build/building/multi-platform/
SUPPORTED_PLATFORMS="linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64"

# Refereces about the docker buildx:
# https://docs.docker.com/build/building/multi-platform/
# https://docs.docker.com/reference/cli/docker/buildx/build
# https://unix.stackexchange.com/questions/748633/error-multiple-platforms-feature-is-currently-not-supported-for-docker-driver
# https://collabnix.com/error-multiple-platforms-feature-is-currently-not-supported-for-docker-driver/
# https://gist.github.com/brianjbayer/d80f60fc6084e77e049f0bb442449519
# https://medium.com/@delphinenyaboke/error-multi-platform-build-is-not-supported-for-the-docker-driver-8883a7a26472
# https://stackoverflow.com/questions/64403659/docker-buildx-image-not-showing-in-docker-image-ls

#----------------------------------------------------------------------------------------------------------




#---------------------------
#---------------------------
# MAIN
#---------------------------
#---------------------------

# References
# https://ryanstutorials.net/bash-scripting-tutorial/bash-input.php
# https://stackoverflow.com/questions/3743793/makefile-why-is-the-read-command-not-reading-the-user-input
# https://stackoverflow.com/questions/60147129/interactive-input-of-a-makefile-variable
# https://makefiletutorial.com/
# https://stackoverflow.com/questions/589276/how-can-i-use-bash-syntax-in-makefile-targets
# https://til.hashrocket.com/posts/k3kjqxtppx-escape-dollar-sign-on-makefiles
# https://stackoverflow.com/questions/5618615/check-if-a-program-exists-from-a-makefile
# https://www.docker.com/blog/multi-arch-build-and-images-the-simple-way/


requirements:
REQUIRED_PACKAGES := docker go
$(foreach package,$(REQUIRED_PACKAGES),\
	$(if $(shell command -v $(package) 2> /dev/null),$(info Found `$(package)`),$(error Please install `$(package)`)))

build:
  # Updating go dependecies
	go get -u
	go mod download
	CGO_ENABLED=0 GOOS=linux go build -o ${BIN_FILE}
	./${BIN_FILE}

clean:
	rm ${BIN_FILE}

image:
	make requirements
	if [ ! -f ${PATH_DOCKERFILE} ]; then
		echo "[ERROR] File not found: ${PATH_DOCKERFILE}"
		exit 1
	fi

	read -rp 'Username of Docker Hub: ' DOCKER_HUB_ACCOUNT
	read -rsp 'Password of Docker Hub: ' DOCKER_HUB_PASSWORD
	docker login -u "$${DOCKER_HUB_ACCOUNT}" -p "$${DOCKER_HUB_PASSWORD}"

	docker buildx create --use --platform="${SUPPORTED_PLATFORMS}" --name multi-platform-builder
	docker buildx build --push --platform="${SUPPORTED_PLATFORMS}" -t "$${DOCKER_HUB_ACCOUNT}/${APP_NAME}:${VERSION}" .
	docker buildx build --push --platform="${SUPPORTED_PLATFORMS}" -t "$${DOCKER_HUB_ACCOUNT}/${APP_NAME}:latest" .
	mkdir /tmp/caches
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v /tmp/caches:/root/.cache/ aquasec/trivy image "$${DOCKER_HUB_ACCOUNT}/${APP_NAME}:${VERSION}"

up:
	make requirements
	docker run -d -p 8080:8080 --rm --name gofipe "aeciopires/${APP_NAME}:${VERSION}"
	make logs

down:
	docker stop gofipe

logs:
	docker logs -f gofipe

.ONESHELL:
recreate:
	make requirements
	make down
	make up
