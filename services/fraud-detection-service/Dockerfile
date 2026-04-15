FROM python:3.12-slim
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
    && python -c "from opentelemetry.instrumentation.flask import FlaskInstrumentor; print('FlaskInstrumentor import OK')"

COPY app.py .

ENV PYTHONUNBUFFERED=1

EXPOSE 3000

CMD ["python", "app.py"]
