FROM nvidia/cuda@sha256:4ff859525f99de5782aa73607ce24219b07dddd48d12b97c1c301d7e1cfb0a87 AS build

ARG LLAMA_REVISION=86b351fd64d5ebbf1ba795ffd60c8f4a8c958613
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    git cmake build-essential ca-certificates libssl-dev libcurl4-openssl-dev
RUN git init /src && git -C /src remote add origin https://github.com/ggml-org/llama.cpp.git \
    && git -C /src fetch --depth 1 origin "$LLAMA_REVISION" \
    && git -C /src checkout --detach "$LLAMA_REVISION" \
    && test "$(git -C /src rev-parse HEAD)" = "$LLAMA_REVISION"
RUN cmake -S /src -B /src/build -DGGML_CUDA=ON -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_CUDA_ARCHITECTURES=120a -DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF \
    && cmake --build /src/build -j 4 --target llama-server \
    && mkdir -p /out \
    && cp /src/build/bin/llama-server /out/llama-server \
    && printf '%s\n' "$LLAMA_REVISION" > /out/source-revision

FROM nvidia/cuda@sha256:4ff859525f99de5782aa73607ce24219b07dddd48d12b97c1c301d7e1cfb0a87
LABEL org.opencontainers.image.source="https://github.com/maximilienGilet/sovereign-kit" \
      org.opencontainers.image.description="Sovereign Kit llama.cpp CUDA 13.3 / Blackwell 120a / native MTP; no models or credentials"
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    python3 curl ca-certificates openssh-server util-linux libcurl4 libgomp1 \
    && rm -f /etc/ssh/ssh_host_*_key /etc/ssh/ssh_host_*_key.pub \
    && mkdir -p /workspace /run/sshd
COPY --from=build /out/ /opt/sovereign-kit/llama/
COPY smoke.py /opt/sovereign-kit/llama/smoke.py
COPY entrypoint.sh /opt/sovereign-kit/llama/entrypoint.sh
COPY sshd-wrapper.sh /opt/sovereign-kit/llama/sshd-wrapper.sh
COPY install-sshd-wrapper.sh /opt/sovereign-kit/llama/install-sshd-wrapper.sh
RUN sh /opt/sovereign-kit/llama/install-sshd-wrapper.sh
RUN chmod 755 /opt/sovereign-kit/llama/entrypoint.sh \
    && test -x /opt/nvidia/nvidia_entrypoint.sh
RUN python3 /opt/sovereign-kit/llama/smoke.py --cpu-only
WORKDIR /workspace
ENTRYPOINT ["/opt/sovereign-kit/llama/entrypoint.sh"]
CMD ["/bin/bash"]
