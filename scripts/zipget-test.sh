#!/bin/sh

bin=$(dirname "$0")/../bin

$bin/zipget $@ -u - <<END
https://informburo.kz/storage/photos/94/main/Oh38ooVAU32ULU9c7rdUWZU2QoP4ojmlHrabANpL.jpg
https://cdnn21.img.ria.ru/images/151222/66/1512226641_0:52:1920:1132_1920x0_80_0_0_662a2d239f962831c11b351c9a8b9d8b.jpg
https://media.istockphoto.com/id/495721707/ru/фото/снежный-барс.jpg?s=1024x1024&w=is&k=20&c=CI7tOjOj5UjWpo35lNe31fKpGKeT8UUgsvdT51GTmQA=
END
