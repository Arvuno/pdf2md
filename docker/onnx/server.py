#!/usr/bin/env python3
"""PP-DocLayoutV3 ONNX layout detection server.

POST /detect with JSON {"image": "<base64>"} returns layout blocks.
Model is loaded from /model/ (mounted volume).
"""

import base64
import io
import json
import os
import sys

import numpy as np
from flask import Flask, request, jsonify
from PIL import Image

MODEL_DIR = os.environ.get("MODEL_DIR", "/model")
MODEL_PATH = os.path.join(MODEL_DIR, "PP-DocLayoutV3.onnx")
CONFIG_PATH = os.path.join(MODEL_DIR, "config.json")

INPUT_SIZE = 800
IMAGENET_MEAN = np.array([0.485, 0.456, 0.406], dtype=np.float32)
IMAGENET_STD = np.array([0.229, 0.224, 0.225], dtype=np.float32)
CONF_THRESHOLD = 0.5

app = Flask(__name__)

# Global session and labels (loaded at startup)
session = None
labels = []


def load_labels(path: str) -> list[str]:
    with open(path) as f:
        cfg = json.load(f)
    return cfg["label_list"]


def preprocess(img: Image.Image) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    orig_w, orig_h = img.size
    scale_w = INPUT_SIZE / orig_w
    scale_h = INPUT_SIZE / orig_h

    img_resized = img.resize((INPUT_SIZE, INPUT_SIZE), Image.BILINEAR)
    arr = np.array(img_resized, dtype=np.float32) / 255.0
    arr = (arr - IMAGENET_MEAN) / IMAGENET_STD
    arr = np.transpose(arr, (2, 0, 1))
    arr = np.expand_dims(arr, axis=0).astype(np.float32)

    im_shape = np.array([[float(INPUT_SIZE), float(INPUT_SIZE)]], dtype=np.float32)
    scale_factor = np.array([[scale_h, scale_w]], dtype=np.float32)

    return arr, im_shape, scale_factor


def postprocess(raw: np.ndarray, labels: list[str], conf_thresh: float) -> list[dict]:
    blocks = []
    for row in raw:
        label_idx = int(row[0])
        score = float(row[1])
        x1, y1, x2, y2 = float(row[2]), float(row[3]), float(row[4]), float(row[5])
        read_order = int(row[6])

        if score < conf_thresh:
            continue
        if label_idx < 0 or label_idx >= len(labels):
            continue

        blocks.append({
            "Label": labels[label_idx],
            "Confidence": round(score, 4),
            "BBox": [x1, y1, x2, y2],
            "ReadOrder": read_order,
        })

    blocks.sort(key=lambda b: (
        (b["ReadOrder"], 0, 0) if b["ReadOrder"] != 0
        else (99999, b["BBox"][1], b["BBox"][0])
    ))
    return blocks


@app.route("/detect", methods=["POST"])
def detect():
    data = request.get_json()
    if not data or "image" not in data:
        return jsonify({"error": "missing 'image' field"}), 400

    try:
        img_bytes = base64.b64decode(data["image"])
        img = Image.open(io.BytesIO(img_bytes)).convert("RGB")
    except Exception as e:
        return jsonify({"error": f"invalid image: {e}"}), 400

    try:
        img_data, im_shape, scale_factor = preprocess(img)
        outputs = session.run(None, {
            "im_shape": im_shape,
            "image": img_data,
            "scale_factor": scale_factor,
        })
        blocks = postprocess(outputs[0], labels, CONF_THRESHOLD)
        return jsonify(blocks)
    except Exception as e:
        return jsonify({"error": f"inference failed: {e}"}), 500


@app.route("/health")
def health():
    return {"status": "ok"}


if __name__ == "__main__":
    import onnxruntime as ort

    if not os.path.exists(MODEL_PATH):
        print(f"Model not found at {MODEL_PATH}", file=sys.stderr)
        sys.exit(1)

    print(f"Loading ONNX model from {MODEL_PATH}", flush=True)
    labels = load_labels(CONFIG_PATH)
    print(f"Loaded {len(labels)} labels", flush=True)

    providers = ["CUDAExecutionProvider", "CPUExecutionProvider"]
    session = ort.InferenceSession(MODEL_PATH, providers=providers)

    used_provider = session.get_providers()[0]
    print(f"Using provider: {used_provider}", flush=True)

    app.run(host="0.0.0.0", port=5000)
