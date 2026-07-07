# Yokai ComfyUI Docker Image

Multi-architecture (amd64/arm64) Docker image for [ComfyUI](https://github.com/comfyanonymous/ComfyUI) with CUDA support and [ComfyUI-Manager](https://github.com/ltdrdata/ComfyUI-Manager) pre-installed.

## Image

```
spencerbull/yokai-comfyui:latest
```

Published to Docker Hub on every push to `main` that modifies `docker/comfyui/**`.

## Build locally

```bash
docker build -t yokai-comfyui docker/comfyui
```

For multi-arch (requires `docker buildx`):

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t yokai-comfyui docker/comfyui
```

## Run

```bash
docker run --gpus all -p 8188:8188 spencerbull/yokai-comfyui:latest
```

The ComfyUI web interface will be available at `http://localhost:8188`.

## What's included

- **Base**: `nvidia/cuda:12.8.0-runtime-ubuntu24.04`
- **PyTorch** with CUDA support (works on both amd64 and arm64)
- **ComfyUI** (latest from GitHub)
- **ComfyUI-Manager** for easy node/model management
