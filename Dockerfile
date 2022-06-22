FROM golang:1.18-bullseye

RUN go install github.com/beego/bee/v2@latest

ENV APP_HOME /go/src/aiomaf
RUN mkdir -p "$APP_HOME"

WORKDIR "$APP_HOME"
EXPOSE 8080
CMD ["bee", "run"]

VOLUME /go/src/aiomaf/build