# mocki — единый статический бинарь в scratch (~7 МБ).

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mocki ./cmd/mocki

FROM scratch
COPY --from=build /out/mocki /mocki
EXPOSE 3000
ENTRYPOINT ["/mocki"]
CMD ["serve", "/data"]
