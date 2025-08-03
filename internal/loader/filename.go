package loader

import (
	"strconv"
	"strings"
	"unicode"
)

const (
	defaultFileName      = "unnamed"
	maxBaseNameLen       = 100
)

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true,
	"COM4": true, "COM5": true, "COM6": true,
	"COM7": true, "COM8": true, "COM9": true,
	"COM¹": true, "COM²": true, "COM³": true,
	"LPT1": true, "LPT2": true, "LPT3": true,
	"LPT4": true, "LPT5": true, "LPT6": true,
	"LPT7": true, "LPT8": true, "LPT9": true,
	"LPT¹": true, "LPT²": true, "LPT³": true,
}

// constructFileName строит безопасное имя файла:
//
//   - обрезает путь;
//   - заменяет расширение на заданное;
//   - если uniqueNum > 0, базовое имя дополняется суффиксом '-<uniqueNum>';
//   - удаляет управляющие и неграфические символы;
//   - заменяет запрещенные и проблемные символы на '-';
//   - последовательные '-' заменяются на один;
//   - лидирующие и финальные '-' удаляются;
//   - зарезервированные имена windows дополняются символом подчеркивания.
//
// Примеры:
//
//	"/some/path/file.txt", ".png", 0 -> "file.png"
//	"C:\\some\\path\\file.txt", ".png", 0 -> "file.png"
//	"file.txt", ".png", 123 -> "file-123.png"
//	"file<>end", ".png", 0 -> "file-end.png"
//	"con..txt", ".png", 0 -> "con_.png"
//
func constructFileName(fileName string, fileExt string, uniqueNum int) string {
	if fileName == "" {
		if uniqueNum > 0 {
			return defaultFileName + "-" + strconv.Itoa(uniqueNum) + fileExt
		}
		return defaultFileName + fileExt
	}

	// Удалить путь
	if p := strings.LastIndexAny(fileName, `/\`); p != -1 {
		fileName = fileName[p+1:]
	}

	// Удалить расширение (всё после последней точки)
	if p := strings.LastIndexByte(fileName, '.'); p != -1 {
		fileName = fileName[:p]
	}

	baseName := sanitizeFilename(fileName, maxBaseNameLen)

	if uniqueNum > 0 {
		return baseName + "-" + strconv.Itoa(uniqueNum) + fileExt
	}

	// Защита от зарезервированных имён Windows
	if reservedNames[strings.ToUpper(baseName)] {
		baseName = baseName + "_"
	}

	return baseName + fileExt
}

// ASCII опасные символы
const asciiProblem = `<>:"/\|?*~.;#$%&'(){}[]!` + "`"

// Fullwidth неопасные символы, но вводят в заблуждение
const fullwidthProblem = "＜＞：＂／＼｜？＊～；＃＄％＆＇（）｛｝［］！"


func sanitizeFilename(s string, maxLen int) string {
	var sb strings.Builder
	sb.Grow(maxLen)

	prev := '-' // чтобы не писать лидирующий '-'
	n := 0
loop:
	for _, r := range s {
		if n >= maxLen {
			break
		}

		switch {
		case unicode.IsSpace(r):
			// Заменяем пробельные символы на '-'
			r = '-'
		case unicode.IsControl(r) || !unicode.IsPrint(r):
			// Удаляем управляющме и неграфические символы
			continue loop
		case strings.ContainsRune(asciiProblem, r):
			// Заменяем запрещенные и проблемные символы на '-':
			r = '-'
		case strings.ContainsRune(fullwidthProblem, r):
			// Заменяем похожие на запрещенные и проблемные символы на '-':
			r = '-'
		}

		// Схлопываем последовательные '-'
		if r == '-' && prev == '-' {
			continue
		}

		sb.WriteRune(r)
		prev = r
		n++
	}

	name := sb.String()

	// обрезаем финальный '-'
	if n := len(name); n > 0 && name[n-1] == '-' {
		name = name[:n-1]
	}

	if name == "" {
		return defaultFileName
	}

	return name
}
