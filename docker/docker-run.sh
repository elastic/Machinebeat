docker run -d \
  --name=machinebeat \
  --user=root \
  elastic/machinebeat:7.17.0 /go/src/github.com/elastic/machinebeat/machinebeat -e