FROM python:3-alpine

RUN apk add --no-cache iputils-ping wget

RUN wget -qO /usr/local/bin/peerfinder-agent.py "https://peerfinder.dn42.dev/agent/peerfinder-agent.py" \
    && chmod 0755 /usr/local/bin/peerfinder-agent.py

ENTRYPOINT ["python", "/usr/local/bin/peerfinder-agent.py"]
