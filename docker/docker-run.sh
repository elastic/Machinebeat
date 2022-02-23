docker run -d \
  --name=machinebeat \
  --user=root \
  elastic/machinebeat:latest /go/src/github.com/elastic/machinebeat/machinebeat -e