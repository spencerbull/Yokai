FROM nvidia/cuda:12.8.0-runtime-ubuntu24.04

RUN apt-get update && apt-get install -y python3 python3-pip python3-venv git wget && rm -rf /var/lib/apt/lists/*

WORKDIR /app
RUN git clone https://github.com/comfyanonymous/ComfyUI.git .

# Install PyTorch with CUDA - works on both amd64 and arm64
RUN pip3 install --break-system-packages torch torchvision torchaudio

# Install ComfyUI requirements
RUN pip3 install --break-system-packages -r requirements.txt

# Install ComfyUI-Manager
RUN cd custom_nodes && git clone https://github.com/ltdrdata/ComfyUI-Manager.git

EXPOSE 8188
CMD ["python3", "main.py", "--listen", "0.0.0.0", "--port", "8188"]
