FROM ghcr.io/webassembly/wasi-sdk:wasi-sdk-25 AS base

RUN apt-get update && apt-get install -y binaryen curl patch

WORKDIR /re2
COPY buildtools/wasm/version.txt .
RUN curl -L https://github.com/google/re2/archive/refs/tags/$(cat version.txt | awk '{$1=$1};1').tar.gz | tar -xz --strip-components 1 -C /re2
RUN mkdir -p abseil-cpp && curl -L https://github.com/abseil/abseil-cpp/archive/refs/tags/20240722.0.tar.gz | tar -xz --strip-components 1 -C abseil-cpp

COPY buildtools/wasm/patch-absl.txt .
COPY buildtools/wasm/patch-re2.txt .

RUN patch -p1 < patch-re2.txt
RUN cd abseil-cpp && patch -p1 < ../patch-absl.txt

ENV CMAKE_TOOLCHAIN_FILE="/opt/wasi-sdk/share/cmake/wasi-sdk-pthread.cmake"
ENV CFLAGS="-O3 -pthread --target=wasm32-wasip1-threads --sysroot=/opt/wasi-sdk/share/wasi-sysroot -DABSL_FORCE_THREAD_IDENTITY_MODE=1 -D_WASI_EMULATED_MMAN -D_WASI_EMULATED_PROCESS_CLOCKS -D_WASI_EMULATED_SIGNAL -DABSL_HAVE_MMAP"
ENV CXXFLAGS="-fno-exceptions $CFLAGS"

RUN cmake -DCMAKE_BUILD_TYPE=Release -DCMAKE_C_FLAGS="$CFLAGS" -DCMAKE_CXX_FLAGS="$CXXFLAGS" -S . -B build
RUN cmake --build build --target re2 -j 2

WORKDIR /cre2
ADD internal/cre2/cre2.cpp /cre2
ADD internal/cre2/cre2.h /cre2
# Just one source file so not worth running make
RUN $CXX -c cre2.cpp -o cre2.o -I. -I/re2 -I/re2/abseil-cpp $CXXFLAGS \
  && $AR cru libcre2.a cre2.o \
  && $RANLIB libcre2.a

ENV LDFLAGS="-O3 --target=wasm32-wasip1-threads --sysroot=/opt/wasi-sdk/share/wasi-sysroot -mexec-model=reactor -pthread -lwasi-emulated-process-clocks -lwasi-emulated-mman -lwasi-emulated-signal -Wl,--max-memory=4294967296 -Wl,--global-base=1024"

# Separate step so exports can be updated without recompiling.
# Number of layers isn't really a concern for this image.
# global-base=1024 same as emcc and allows further wasm-opt optimizations
RUN $CXX -o libcre2-noopt.so -Wl,--global-base=1024 $LDFLAGS \
  /cre2/libcre2.a \
  /re2/build/libre2.a \
  /re2/build/abseil-cpp/absl/*/*.a \
  -Wl,--import-memory -Wl,--export-memory -Wl,--max-memory=4294967296 \
  -Wl,--export=malloc \
  -Wl,--export=free \
  -Wl,--export=cre2_new \
  -Wl,--export=cre2_delete \
  -Wl,--export=cre2_opt_new \
  -Wl,--export=cre2_opt_delete \
  -Wl,--export=cre2_opt_set_max_mem \
  -Wl,--export=cre2_opt_set_log_errors \
  -Wl,--export=cre2_opt_set_longest_match \
  -Wl,--export=cre2_opt_set_posix_syntax \
  -Wl,--export=cre2_opt_set_case_sensitive \
  -Wl,--export=cre2_opt_set_latin1_encoding \
  -Wl,--export=cre2_error_code \
  -Wl,--export=cre2_error_arg \
  -Wl,--export=cre2_num_capturing_groups \
  -Wl,--export=cre2_match \
  -Wl,--export=cre2_named_groups_iter_new \
  -Wl,--export=cre2_named_groups_iter_next \
  -Wl,--export=cre2_named_groups_iter_delete \
  -Wl,--export=cre2_set_new \
  -Wl,--export=cre2_set_add \
  -Wl,--export=cre2_set_match \
  -Wl,--export=cre2_set_delete \
  -Wl,--export=cre2_set_compile \
  -Wl,--export=__wasm_init_tls \
  -Wl,--export=__stack_pointer \
  -Wl,--export=__tls_base

RUN wasm-opt -o libcre2.wasm --low-memory-unused --flatten --rereloop --converge -O3 libcre2-noopt.so

CMD ["cp", "libcre2.wasm", "/out/"]
