<a id="top"></a>
<div align="center">

<img src="assets/banner.svg" alt="mocki" width="900"/>

[![CI](https://img.shields.io/github/actions/workflow/status/suggydev/mocki/ci.yml?branch=main&style=for-the-badge&label=CI)](https://github.com/suggydev/mocki/actions)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-FF6D00?style=for-the-badge)](LICENSE)

**Мгновенный mock REST API из JSON-файлов. Наследник json-server — но единый статический бинарь на чистом stdlib.**

</div>

---

## Быстрый старт

```bash
go install github.com/suggydev/mocki/cmd/mocki@latest
mocki serve examples          # → http://localhost:3000

# или в Docker (~7 МБ, scratch)
docker build -t mocki . && docker run -p 3000:3000 -v $PWD/examples:/data mocki
```

Файл `users.json` → ресурс `/users`. Файл `db.json` вида `{"users": [...], "posts": [...]}` → ресурсы по ключам.

## REST семантика

```bash
curl 'localhost:3000/users'                      # список
curl 'localhost:3000/users/1'                    # запись
curl 'localhost:3000/users?role=admin'           # фильтр field=value
curl 'localhost:3000/users?q=ada'                # подстрока по строковым полям
curl 'localhost:3000/users?_sort=age&_order=desc' # сортировка
curl 'localhost:3000/users?_page=2&_limit=10'    # пагинация (+ X-Total-Count)
curl -X POST localhost:3000/users -d '{"name":"Dan"}'   # create (авто-id)
curl -X PATCH localhost:3000/users/1 -d '{"age":37}'    # patch
curl -X PUT localhost:3000/users/1 -d '{"name":"Ada"}'  # replace
curl -X DELETE localhost:3000/users/1                   # delete
```

## Флаги

```
mocki serve <dir|file.json> [flags]     # флаги в любом порядке
  -p, --port N     порт (по умолч. 3000)
      --latency D  задержка ответа, напр. 200ms — симуляция медленной сети
      --no-cors    выключить CORS (включён по умолчанию)
      --no-watch   выключить hot reload (включён: mtime-polling 500ms, zero-dep)
```

## Почему не json-server

- **json-server v1** годами в альфе; **mocki** — один статический бинарь, zero npm-зависимостей
- Чистый **stdlib Go**: ни одной внешней зависимости в go.mod
- Hot reload без fsnotify: mtime-polling — работает везде, включая scratch-контейнер

## Разработка

```bash
go test ./...    # unit (store: фильтры/пагинация/CRUD/reload) + httptest (server)
go run ./cmd/mocki serve examples
```

## License

MIT — see [LICENSE](LICENSE).
