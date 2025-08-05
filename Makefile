# Определяем директорию для бинарников
BIN_DIR := bin

# Кастомные флаги сборки (можно переопределить при вызове make)
GO_BUILD_FLAGS ?=

# Находим все поддиректории в cmd, которые потенциально могут быть бинарниками
CMDS := $(wildcard cmd/*)

# Генерируем список целей для бинарников
BINARIES := $(patsubst cmd/%,$(BIN_DIR)/%,$(CMDS))


# Основная цель - собирает все бинарники
all: deps $(BINARIES)

# Правило для подготовки зависимостей
deps:
	go mod tidy
	
# Шаблонное правило для сборки любого бинарника
$(BIN_DIR)/%: FORCE
	@mkdir -p $(@D)
	go build $(GO_BUILD_FLAGS) -o $@ ./cmd/$(notdir $@)

# Очистка
clean:
	rm -rf $(BIN_DIR)


.PHONY: all clean FORCE

FORCE:
