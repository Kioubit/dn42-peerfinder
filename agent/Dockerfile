FROM python:3-alpine

RUN apk add --no-cache iputils-ping

COPY --chmod=0755 peerfinder-agent.py /usr/local/bin/peerfinder-agent.py

EXPOSE 9000

ENTRYPOINT ["python", "/usr/local/bin/peerfinder-agent.py"]
