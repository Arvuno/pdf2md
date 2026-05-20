FROM nvidia/cuda:12.9.2-cudnn-runtime-ubuntu24.04

RUN apt-get update -qq && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
        python3 python3-pip libgl1 libglib2.0-0 && \
    pip3 install --no-cache-dir --break-system-packages \
        onnxruntime-gpu numpy pillow flask && \
    rm -rf /var/lib/apt/lists/*


COPY server.py /opt/server.py

EXPOSE 5000
ENTRYPOINT ["python3", "/opt/server.py"]
