FROM alpine:3.22

RUN apk --no-cache upgrade
RUN apk --no-cache add ca-certificates tzdata zsh jq tmux curl

RUN addgroup -g 1000 ntpmon && adduser -u 1000 -D -G ntpmon ntpmon
RUN touch ~ntpmon/.zshrc ~root/.zshrc; chown ntpmon:ntpmon ~ntpmon/.zshrc

RUN mkdir /app
ADD dist/ntppool-agent_linux_amd64_v1/ntppool-agent /app/
ADD dist/monitor-api_linux_amd64_v2/monitor-api /app/
ADD dist/monitor-scorer_linux_amd64_v2/monitor-scorer /app/

EXPOSE 8000
EXPOSE 8080

USER ntpmon

# Container start command for production
CMD ["/app/monitor-api"]
