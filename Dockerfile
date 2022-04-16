################ Production ################
#FROM harbor.ntppool.org/ntppool/base-os:3.15.4 as production
FROM alpine:3.15.4

ADD dist/monitor-api_linux_amd64/monitor-api /app
ADD dist/ntppool-monitor_linux_amd64/monitor /app

EXPOSE 8000
EXPOSE 8080

# Container start command for production
CMD ["/app/monitor-api"]
