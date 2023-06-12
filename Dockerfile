FROM alpine:3.18.0

RUN apk --no-cache upgrade
RUN apk --no-cache add ca-certificates tzdata zsh jq tmux curl

RUN addgroup -g 1000 ntpmon && adduser -u 1000 -D -G ntpmon ntpmon
RUN touch ~ntpmon/.zshrc ~root/.zshrc; chown ntpmon:ntpmon ~ntpmon/.zshrc

RUN mkdir /app
ADD dist/monitor-api_linux_amd64_v1/monitor-api /app/
ADD dist/ntppool-monitor_linux_amd64_v1/ntppool-monitor /app/
ADD dist/monitor-scorer_linux_amd64_v1/monitor-scorer /app/

EXPOSE 8000
EXPOSE 8080

USER ntpmon

# Container start command for production
CMD ["/app/monitor-api"]
