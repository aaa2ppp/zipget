FROM alpine AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
# Сертификаты CA и TLS-сертификат приложения
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Бинарник приложения
COPY ./bin/app /bin/app

# Безопасность: не-root пользователь
USER 65535:65535

ENTRYPOINT [ "/bin/app" ]
