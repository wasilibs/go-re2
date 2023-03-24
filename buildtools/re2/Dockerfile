# Copyright 2022 The OWASP Coraza contributors
# SPDX-License-Identifier: Apache-2.0

FROM ghcr.io/corazawaf/coraza-proxy-wasm/buildtools-wasi-sdk:main

RUN apt-get update && apt-get install -y binaryen

# Use post-release commit for now as it includes support for no-threads
RUN mkdir -p /re2 && curl -L https://github.com/google/re2/archive/954656f47fe8fb505d4818da1e128417a79ea500.tar.gz | tar -xz --strip-components 1 -C /re2
WORKDIR /re2
ENV RE2_CXXFLAGS -Wall -Wextra -Wno-unused-parameter -Wno-missing-field-initializers -I. -DRE2_NO_THREADS
RUN make obj/libre2.a

WORKDIR /cre2
ADD internal/cre2/cre2.cpp /cre2
ADD internal/cre2/cre2.h /cre2
# Just one source file so not worth running make
RUN $CXX -c cre2.cpp -o cre2.o -I. -I/re2 $CXXFLAGS && \
    llvm-ar-15 cru libcre2.a  cre2.o && \
    llvm-ranlib-15 libcre2.a

# Separate step so exports can be updated without recompiling.
# Number of layers isn't really a concern for this image.
# global-base=1024 same as emcc and allows further wasm-opt optimizations
RUN $CXX -o libcre2-noopt.so -Wl,--global-base=1024 -mexec-model=reactor --rtlib=compiler-rt --target=wasm32-wasi -shared \
    -nostdlib /wasi-sysroot/lib/wasm32-wasi/crt1-reactor.o \
    /usr/lib/llvm-15/lib/clang/15.*/lib/wasi/libclang_rt.builtins-wasm32.a \
    /re2/obj/libre2.a \
    /cre2/libcre2.a \
    -L/wasi-sysroot/lib/wasm32-wasi -lc++ -lc++abi -lc \
    --sysroot=/wasi-sysroot -Wl,--demangle -Wl,--allow-undefined \
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
    -Wl,--export=cre2_opt_set_encoding \
    -Wl,--export=cre2_error_code \
    -Wl,--export=cre2_error_arg \
    -Wl,--export=cre2_num_capturing_groups \
    -Wl,--export=cre2_match \
    -Wl,--export=cre2_named_groups_iter_new \
    -Wl,--export=cre2_named_groups_iter_next \
    -Wl,--export=cre2_named_groups_iter_delete \
    -Wl,--export=cre2_global_replace_re

RUN wasm-opt -o libcre2.so --low-memory-unused --flatten --rereloop --converge -O3 libcre2-noopt.so

CMD ["bash", "-c", "cp /re2/obj/libre2.a /cre2/libcre2.so /cre2/libcre2.a /out/"]
