## WebDAV сервер с версионированием

### Запуск
Достаточно просто в папке `/server` собрать сервер и запустить, конфигурировать можно все через переменные окружения, флаги или файл `config.json`. Подбробнее о параметрах можно прочитать в коде или в отчете.
```
go build . && ./webdavvc
{"level":"info","ts":1684769638.2765195,"caller":"server/main.go:42","msg":"Starting HTTP server","addr":5555}
```
Сервер напишет лог о том, что он запустился

### Примеры запросов
* HTTP
    ```
    curl -i -X PUT localhost:5555/a.xml -d '{"key1":"value1", "key2":"value2"}'
    HTTP/1.1 201 Created
    Etag: "1761817711f55b2c22"
    Date: Mon, 22 May 2023 15:41:24 GMT
    Content-Length: 7
    Content-Type: text/plain; charset=utf-8

    Created
    ```

    ```
    curl -i -X GET localhost:5555/a.xml -d '{"key1":"value1", "key2":"value2"}'
    HTTP/1.1 200 OK
    Accept-Ranges: bytes
    Content-Length: 34
    Content-Type: text/xml; charset=utf-8
    Etag: "1761817711f55b2c22"
    Last-Modified: Mon, 22 May 2023 15:41:24 GMT
    Date: Mon, 22 May 2023 15:41:35 GMT

    {"key1":"value1", "key2":"value2"}
    ```
* WebDAV
    ```
    curl -i -X MKCOL localhost:5555/dir
    HTTP/1.1 201 Created
    Date: Mon, 22 May 2023 15:43:23 GMT
    Content-Length: 7
    Content-Type: text/plain; charset=utf-8

    Created
    ```

    ```
    curl -i -X LOCK localhost:5555/a.xml -d @lock.xml
    HTTP/1.1 200 OK
    Content-Type: application/xml; charset=utf-8
    Lock-Token: <1684769643>
    Date: Mon, 22 May 2023 15:45:47 GMT
    Content-Length: 471

    <?xml version="1.0" encoding="utf-8"?>
    <D:prop xmlns:D="DAV:"><D:lockdiscovery><D:activelock>
            <D:locktype><D:write/></D:locktype>
            <D:lockscope><D:exclusive/></D:lockscope>
            <D:depth>infinity</D:depth>
            <D:owner>                                                   <D:href>http://jpepper.com</D:href>                                                         </D:owner>
            <D:timeout>Second-0</D:timeout>
            <D:locktoken><D:href>1684769643</D:href></D:locktoken>
            <D:lockroot><D:href>a.xml</D:href></D:lockroot>
    </D:activelock></D:lockdiscovery></D:prop>
    ```

    ```
    curl -i -X UNLOCK localhost:5555/a.xml -H "Lock-Token: <1684769643>"
    HTTP/1.1 204 No Content
    Date: Mon, 22 May 2023 15:45:58 GMT
    ```

* WebDAV with VC
    ```
    > curl -i -X VERSION-CONTROL localhost:5555/a.xml
    HTTP/1.1 200 OK
    Etag: "176181ea94554e2922"
    Version: a10430e9f205423f6118a3222a785956ca681bfc
    Date: Mon, 22 May 2023 15:49:45 GMT
    Content-Length: 2
    Content-Type: text/plain; charset=utf-8

    OK
    > curl -i -X CHECKIN localhost:5555/a.xml
    HTTP/1.1 201 Created
    Etag: "176181ea94554e2922"
    Version: 811d42eb1c81a90e2733f839c2b5856075e3b173
    Date: Mon, 22 May 2023 15:49:49 GMT
    Content-Length: 7
    Content-Type: text/plain; charset=utf-8

    Created
    > curl -i -X CHECKOUT localhost:5555/a.xml -H
        "Version:a10430e9f205423f6118a3222a785956ca681bfc"
    HTTP/1.1 200 OK
    Etag: "176181ea94554e2922"
    Date: Mon, 22 May 2023 15:50:10 GMT
    Content-Length: 2
    Content-Type: text/plain; charset=utf-8

    OK
    > curl -i -X CHECKIN localhost:5555/a.xml
    HTTP/1.1 405 Method Not Allowed
    Allow: GET, UNCHECKOUT, VERSION-CONTROL
    Etag: "176181ea94554e2922"
    Date: Mon, 22 May 2023 15:50:16 GMT
    Content-Length: 18
    Content-Type: text/plain; charset=utf-8

    Method Not Allowed
    > curl -i -X UNCHECKOUT localhost:5555/a.xml
    HTTP/1.1 200 OK
    Etag: "176181ea94554e2922"
    Date: Mon, 22 May 2023 15:50:27 GMT
    Content-Length: 2
    Content-Type: text/plain; charset=utf-8

    OK
    > curl -i -X CHECKIN localhost:5555/a.xml
    HTTP/1.1 201 Created
    Etag: "176181ea94554e2922"
    Version: 45614e8ef16af4a40ec6fda674b85bfb9fe529eb
    Date: Mon, 22 May 2023 15:50:33 GMT
    Content-Length: 7
    Content-Type: text/plain; charset=utf-8

    Created
    ```

### Запуск тестов
В папке с тестами можно запустить так. Это стандартные Go тесты, так что все их опции работают
```
cd tests/ && go test -bench=. -benchmem
goos: linux
goarch: amd64
pkg: jpepper_webdav/webdavvc/tests
cpu: Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz
BenchmarkGet-12                             3745            281933 ns/op           21273 B/op        159 allocs/op
BenchmarkVersionControlCacheMiss-12         2217            851548 ns/op           58151 B/op        519 allocs/op
BenchmarkVersionControlCacheHit-12          5250            305916 ns/op           24240 B/op        202 allocs/op
BenchmarkCheckin-12                          644           1665793 ns/op          320706 B/op       1558 allocs/op
BenchmarkCheckout-12                        1116           1089531 ns/op          158541 B/op       1041 allocs/op
PASS
ok      jpepper_webdav/webdavvc/tests   12.183s
```
