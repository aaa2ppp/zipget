#/bin/sh

port=${1:-8080}

LANG=ru_RU.UTF-8

file1='https://informburo.kz/storage/photos/94/main/Oh38ooVAU32ULU9c7rdUWZU2QoP4ojmlHrabANpL.jpg'
file2='https://media.istockphoto.com/id/495721707/ru/фото/снежный-барс.jpg?s=1024x1024&w=is&k=20&c=CI7tOjOj5UjWpo35lNe31fKpGKeT8UUgsvdT51GTmQA='
file3='http://localhost/private/home-sex.jpg'
file4='https://cdnn21.img.ria.ru/images/151222/66/1512226641_0:52:1920:1132_1920x0_80_0_0_662a2d239f962831c11b351c9a8b9d8b.jpg'

tmp_dir="$(dirname "$0")/../tmp"
mkdir -p "$tmp_dir" || exit 1

#curl="curl -v"
curl="curl -fsS"

echo "== Создаём задачу" >&2
TASK_ID=$($curl -X POST http://localhost:${port}/api/tasks | jq -r '.task_id')
echo "TASK_ID: $TASK_ID"

echo "== Добавляем файлы" >&2
# NOTE: на MINGW если предавать тело через параметр, портится кириллица, потому через stdin

jq -n --arg url "$file1" '{url: $url}' |
$curl -X POST -H "Content-Type: application/json" --data-binary @- http://localhost:$port/api/tasks/"$TASK_ID"/files

jq -n --arg url "$file2" '{url: $url}' |
$curl -X POST -H "Content-Type: application/json" --data-binary @- http://localhost:$port/api/tasks/"$TASK_ID"/files

jq -n --arg url "$file3" '{url: $url}' |
$curl -X POST -H "Content-Type: application/json" --data-binary @- http://localhost:$port/api/tasks/"$TASK_ID"/files

echo "== Привышение лимита" >&2
jq -n --arg url "$file4" '{url: $url}' |
$curl -X POST -H "Content-Type: application/json" --data-binary @- http://localhost:$port/api/tasks/"$TASK_ID"/files

echo "== Получаем статус (с ссылкой на архив)" >&2
$curl http://localhost:$port/api/tasks/"$TASK_ID" | jq

echo "== Скачиваем архив" >&2
(
    mkdir -p "$tmp_dir/api" && cd "$tmp_dir/api" && \
    $curl -OJ http://localhost:$port/api/tasks/"$TASK_ID"/archive
)

echo "== Скачиваем архив по прямой ссылке" >&2
(
    mkdir -p "$tmp_dir/files" && cd "$tmp_dir/files" && \
    $curl -L -OJ http://localhost:$port/files/task_"$TASK_ID".zip
)

echo "== Удаляем задачу" >&2
$curl -X DELETE http://localhost:$port/api/tasks/"$TASK_ID"

echo "== Проверяем, что задачи нет" >&2
$curl http://localhost:$port/api/tasks/"$TASK_ID" | jq
