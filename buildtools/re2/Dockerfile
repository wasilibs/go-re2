
FROM ghcr.io/webassembly/wasi-sdk:wasi-sdk-22 AS base

RUN apt-get update && apt-get install -y binaryen curl

WORKDIR /re2
RUN curl -L https://github.com/google/re2/archive/refs/tags/2023-03-01.tar.gz | tar -xz --strip-components 1 -C /re2

ENV CFLAGS --target=wasm32-wasi-threads --sysroot=/wasi-sysroot -pthread -O3 -D__USE_ISOC11 -msimd128
ENV CXXFLAGS -fno-exceptions $CFLAGS
ENV LDFLAGS $CFLAGS -mexec-model=reactor

RUN make obj/libre2.a

WORKDIR /cre2
ADD internal/cre2/cre2.cpp /cre2
ADD internal/cre2/cre2.h /cre2
# Just one source file so not worth running make
RUN $CXX -c cre2.cpp -o cre2.o -I. -I/re2 $CXXFLAGS \
  && $AR cru libcre2.a cre2.o \
  && $RANLIB libcre2.a

# Separate step so exports can be updated without recompiling.
# Number of layers isn't really a concern for this image.
# global-base=1024 same as emcc and allows further wasm-opt optimizations
RUN $CXX -o libcre2-noopt.so -Wl,--global-base=1024 $LDFLAGS \
  /re2/obj/libre2.a \
  /cre2/libcre2.a \
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
  -Wl,--export=__wasm_init_tls \
  -Wl,--export=__stack_pointer \
  -Wl,--export=__tls_base

RUN wasm-opt -o libcre2.so --low-memory-unused --flatten --rereloop --converge -O3 libcre2-noopt.so

CMD ["cp", "libcre2.so", "/out/"]
