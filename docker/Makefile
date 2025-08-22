# Docker orchestration for multiple zipgetd instances
#
# Цели:
#   make up1        # Запустить инстанс 1
#   make up-all     # Запустить все
#   make down1      # Остановить и удалить инстанс 1
#   make down-all   # Остановить и удалить всех
#   make clean      # Очистить всё (образ, бинарник)
#
# Переменные:
#   INSTANCES=3     # Количество инстансов
#   BASE_PORT=8080  # Базовый порт (N → 8080 + N - 1)
#   TAG=latest      # Тег образа
#   ENV=.env        # Файл конфигурации

IMAGE := zipgetd
TAG ?= latest
IMAGE_NAME := $(IMAGE):$(TAG)

# Параметры сборки бинарника
CMD_PATH := ../cmd/zipgetd
BUILD_OPTIONS ?= -ldflags "-s -w" -trimpath

# Конфигурация приложения
ENV ?= ./.env
INTERNAL_PORT := $(shell . $(ENV); echo $${SERVER_ADDR#:})
INTERNAL_PORT ?= 8080

# Параметры контейнеров
BASE_PORT ?= 8080
INSTANCES ?= 3
RUN_OPTIONS ?= --restart=unless-stopped --cpus=0.1 --memory=50m


.PHONY: all build build-image up-all down-all start-all stop-all clean
.PHONY: $(patsubst %,up%, $(shell seq 1 $(INSTANCES)))
.PHONY: $(patsubst %,down%, $(shell seq 1 $(INSTANCES)))
.PHONY: $(patsubst %,start%, $(shell seq 1 $(INSTANCES)))
.PHONY: $(patsubst %,stop%, $(shell seq 1 $(INSTANCES)))

all: up-all

# === Сборка ===
build: .build.done

.build.done:
	-rm -fr ./bin
	GOOS=linux GOARCH=amd64 go build $(BUILD_OPTIONS) -o ./bin/app $(CMD_PATH)
	upx --lzma --force-overwrite -q --no-progress ./bin/app
	@touch $@

build-image: .build-image.done

.build-image.done: .build.done
	docker build -t $(IMAGE_NAME) .
	@touch $@


# === Управление контейнерами ===
# Цели: upN, downN, startN, stopN, где N номер контейнера
# Для изменения количества инстансов — поменяй INSTANCES
define INSTANCE_RULE
up$(1): build-image
	-docker rm $(IMAGE)-$(1)
	docker run \
		--env-file $(ENV) \
		-d $(RUN_OPTIONS) \
		-p $(shell expr $(BASE_PORT) + $(1) - 1):$(INTERNAL_PORT) \
		--name $(IMAGE)-$(1) \
		$(IMAGE):$(TAG)

down$(1):
	-docker stop $(IMAGE)-$(1)
	-docker rm $(IMAGE)-$(1)

start$(1):
	docker start $(IMAGE)-$(1)

stop$(1):
	docker stop $(IMAGE)-$(1)
endef

$(foreach i,$(shell seq 1 $(INSTANCES)),$(eval $(call INSTANCE_RULE,$(i))))


# === Массовое управление контейнерами ===
# Цели: up-all, down-all, start-all, stop-all
define ALL_RULE
$(1)-all: $(patsubst %,$(1)%,$(shell seq 1 $(INSTANCES)))
endef

$(foreach i,up down start stop,$(eval $(call ALL_RULE,$(i))))


# === Очистка ===
clean: down-all
	-docker image rm $(IMAGE_NAME)
	-rm -fr ./bin ./.*.done
