package loader

import (
	"strconv"
	"strings"
	"testing"

	"github.com/nalgeon/be"
)

func TestConstructFileName(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		fileExt   string
		uniqueNum int
		want      string
	}{
		{
			name:     "normal_name",
			fileName: "photo",
			fileExt:  ".jpg",
			want:     "photo.jpg",
		},
		{
			name:     "normal_name_with_ext",
			fileName: "photo.png",
			fileExt:  ".jpg",
			want:     "photo.jpg",
		},
		{
			name:     "empty_filename",
			fileName: "",
			fileExt:  ".txt",
			want:     "unnamed.txt",
		},
		{
			name:      "empty_with_unique",
			fileName:  "",
			fileExt:   ".tmp",
			uniqueNum: 5,
			want:      "unnamed-5.tmp",
		},
		{
			name:     "dangerous_chars",
			fileName: `file<>:"|?*evil`,
			fileExt:  ".exe",
			want:     "file-evil.exe",
		},
		{
			name:     "unicode_dangerous_chars",
			fileName: "file＜＞：＂／＼｜？＊",
			fileExt:  ".pdf",
			want:     "file.pdf",
		},
		{
			name:     "control_chars",
			fileName: "file\x00\x01\x0A\x1Fend",
			fileExt:  ".log",
			want:     "file-end.log",
		},
		{
			name:     "path_unix",
			fileName: "/home/user/virus.exe",
			fileExt:  ".txt",
			want:     "virus.txt",
		},
		{
			name:     "path_windows",
			fileName: `C:\Users\Public\malware.bat`,
			fileExt:  ".js",
			want:     "malware.js",
		},
		{
			name:     "dots_and_spaces",
			fileName: "  ...  filename...txt  ",
			fileExt:  ".zip",
			want:     "filename.zip",
		},
		{
			name:     "reserved_name_CON",
			fileName: "CON",
			fileExt:  ".txt",
			want:     "CON_.txt",
		},
		{
			name:     "reserved_name_COM1",
			fileName: "COM1",
			fileExt:  ".exe",
			want:     "COM1_.exe",
		},
		{
			name:     "reserved_name_LPT9",
			fileName: "LPT9",
			fileExt:  ".dat",
			want:     "LPT9_.dat",
		},
		{
			name:     "reserved_name_mixed_case",
			fileName: "con",
			fileExt:  ".tmp",
			want:     "con_.tmp",
		},
		{
			name:     "long_name",
			fileName: strings.Repeat("a", 150),
			fileExt:  ".bin",
			want:     strings.Repeat("a", 100) + ".bin",
		},
		{
			name:      "long_name_with_unique",
			fileName:  strings.Repeat("b", 95),
			fileExt:   ".tmp",
			uniqueNum: 123,
			want:      strings.Repeat("b", 95) + "-123.tmp",
		},
		{
			name:      "unique_num",
			fileName:  "backup",
			fileExt:   ".tar",
			uniqueNum: 7,
			want:      "backup-7.tar",
		},
		{
			name:     "homoglyph_fullwidth_slash",
			fileName: "file／path／malicious",
			fileExt:  ".js",
			want:     "file-path-malicious.js",
		},
		{
			name:     "homoglyph_colon",
			fileName: "file：secret",
			fileExt:  ".ini",
			want:     "file-secret.ini",
		},
		{
			name:      "only_dots_and_spaces",
			fileName:  ".... . .",
			fileExt:   ".tmp",
			uniqueNum: 0,
			want:      "unnamed.tmp",
		},
		{
			name:     "only_invalid_chars",
			fileName: `<>:"|?*`,
			fileExt:  ".dat",
			want:     "unnamed.dat",
		},
		{
			name:      "reserved_with_unique",
			fileName:  "AUX",
			fileExt:   ".log",
			uniqueNum: 2,
			want:      "AUX-2.log",
		},
		{
			name:     "filename_with_newline",
			fileName: "file\nname",
			fileExt:  ".txt",
			want:     "file-name.txt",
		},
		{
			name:     "filename_with_tab",
			fileName: "file\tname",
			fileExt:  ".csv",
			want:     "file-name.csv",
		},
		{
			name:     "filename_with_ideographic_space",
			fileName: "file　name", // U+3000
			fileExt:  ".md",
			want:     "file-name.md",
		},
		{
			name:     "one_dot",
			fileName: ".",
			want:     "unnamed",
		},
		{
			name:     "two_dots",
			fileName: "..",
			want:     "unnamed",
		},
		{
			name:     "only_dots",
			fileName: ".......................",
			want:     "unnamed",
		},
		{
			name:     "dots_in_middle",
			fileName: "file................end",
			want:     "file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructFileName(tt.fileName, tt.fileExt, tt.uniqueNum)
			be.Equal(t, got, tt.want)
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		// Зарезервированные имена Windows
		{"NUL.tar.gz", "NUL-tar-gz"},
		{"COM1.config", "COM1-config"},

		// Path traversal и инъекции
		{"../../etc/passwd", "etc-passwd"},
		{"C:\\Windows\\System32", "C-Windows-System32"},
		{"file; rm -rf /", "file-rm-rf"},
		{"`reboot`", "reboot"},
		{"$(id)", "id"},
		{"| ls", "ls"},

		// Unicode-атаки
		{"document.\u202Egpj.exe", "document-gpj-exe"},         // BIDI
		{"photo_\u200B\u200Bmalware.jpg", "photo_malware-jpg"}, // Zero-width
		{"paypаl.com", "paypаl-com"},                           // Homoglyph (кириллическая 'а')
		{"\uFF0Fetc\uFF0Fpasswd", "etc-passwd"},                // Fullwidth /
		{"\u2163IV", "\u2163IV"},                               // Римская цифра (не должна фильтроваться)

		// Специальные символы
		{"50%.png", "50-png"},
		{"file$name.txt", "file-name-txt"},
		{"price#1.jpg", "price-1-jpg"},
		{"my&file", "my-file"},
		{"odd!name", "odd-name"},
		{"(config)", "config"},
		{"{settings}", "settings"},
		{"[data]", "data"},
		{"@user", "@user"},
		{"+plus+", "+plus+"},

		// Пробелы и разделители
		{"  trim  me  ", "trim-me"},
		{"tab\tseparated", "tab-separated"},
		{"new\nline", "new-line"},

		// Точки и расширения
		{".hidden", "hidden"},
		{"..config", "config"},
		{"file..name", "file-name"},
		{"double..dots", "double-dots"},

		// Пустые и мусорные значения
		{"", "unnamed"},
		{"...", "unnamed"},
		{"----", "unnamed"},
		{"\x00\x01\x02", "unnamed"},

		// Длинные имена
		{strings.Repeat("a", 300), strings.Repeat("a", maxBaseNameLen)},
		{strings.Repeat("-abc-", 60), strings.Repeat("-abc", maxBaseNameLen/4)[1:]},
		{"a" + strings.Repeat("!", 100) + "b", "a-b"},

		// Экзотические языки
		{"中文文档.txt", "中文文档-txt"},
		{"日本語_ファイル.pdf", "日本語_ファイル-pdf"},
		{"РусскийДокумент.docx", "РусскийДокумент-docx"},
		{"emoji😊file", "emoji😊file"}, // Графические символы
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i+1), func(t *testing.T) {
			got := sanitizeFilename(tt.input, maxBaseNameLen)
			be.Equal(t, got, tt.output)
		})
	}
}

func TestConstructFileName2(t *testing.T) {
	tests := []struct {
		fileName  string
		fileExt   string
		uniqueNum int
		output    string
	}{
		// Примеры
		{"/some/path/file.txt", ".png", 0, "file.png"},
		{"C:\\some\\path\\file.txt", ".png", 0, "file.png"},
		{"file.txt", ".png", 123, "file-123.png"},
		{"file<>end", ".png", 0, "file-end.png"},
		{"con..txt", ".png", 0, "con_.png"},

		// Базовые случаи
		{"file.txt", ".jpg", 0, "file.jpg"},
		{"document.pdf", ".zip", 123, "document-123.zip"},
		{"", ".txt", 0, "unnamed.txt"},
		{"", ".png", 5, "unnamed-5.png"},

		// Windows reserved с uniqueNum
		{"CON", ".txt", 0, "CON_.txt"},
		{"CON.ext", ".txt", 0, "CON_.txt"},
		{"COM1", ".log", 5, "COM1-5.log"}, // С суффиксом разрешено
		{"COM1.ext", ".log", 5, "COM1-5.log"},
		{"COM1.ext.ext", ".log", 5, "COM1-ext-5.log"},
		{"COM1.ext.ext", ".log", 0, "COM1-ext.log"},

		// Path traversal в имени
		{"../../etc/passwd", ".conf", 0, "passwd.conf"},
		{"C:\\Windows\\file", ".dll", 0, "file.dll"},

		// Специальные символы
		{"file;name", ".txt", 0, "file-name.txt"},
		{"data%2023", ".csv", 0, "data-2023.csv"},
		{"price#1", ".json", 0, "price-1.json"},

		// Unicode
		{"文档.\u202Egpj", ".pdf", 0, "文档.pdf"},
		{"فایل", ".txt", 0, "فایل.txt"},

		// Длинные имена с расширениями
		{strings.Repeat("a", 300), ".ext", 0, strings.Repeat("a", maxBaseNameLen) + ".ext"},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i+1), func(t *testing.T) {
			got := constructFileName(tt.fileName, tt.fileExt, tt.uniqueNum)
			be.Equal(t, got, tt.output)
		})
	}
}
