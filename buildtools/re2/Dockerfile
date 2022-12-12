# Copyright 2022 The OWASP Coraza contributors
# SPDX-License-Identifier: Apache-2.0

FROM ghcr.io/corazawaf/coraza-proxy-wasm/buildtools-wasi-sdk:main

RUN apt-get install -y patch

# Use post-release commit for now as it includes support for no-threads
RUN mkdir -p /re2 && curl -L https://github.com/google/re2/archive/954656f47fe8fb505d4818da1e128417a79ea500.tar.gz | tar -xz --strip-components 1 -C /re2
WORKDIR /re2
ENV RE2_CXXFLAGS -Wall -Wextra -Wno-unused-parameter -Wno-missing-field-initializers -I. -DRE2_NO_THREADS
RUN make obj/libre2.a

RUN mkdir -p /cre2 && curl -L https://bitbucket.org/marcomaggi/cre2/downloads/cre2-0.4.0-devel.2.tar.gz | tar -xz --strip-components 1 -C /cre2
WORKDIR /cre2
ADD cre2.patch cre2.patch
RUN patch -p1 < cre2.patch
# host is required by configure but not used so set it arbitrarily
RUN RE2_CFLAGS=-I/re2 RE2_LIBS=/re2/obj ./configure --host=i686-pc-linux-gnu --enable-shared=false && \
    make
# We invoke linker for shared library directly to include flags for embedding all code into a single binary instead of
# dynamic loading. Currently unknown if it's possible to nudge libtool to do this.
RUN clang++-15 -Wl,--no-entry --rtlib=compiler-rt --target=wasm32-wasi -shared -nostdlib /usr/lib/llvm-15/lib/clang/15.*/lib/wasi/libclang_rt.builtins-wasm32.a /re2/obj/libre2.a /cre2/.libs/libcre2.a -L/wasi-sysroot/lib/wasm32-wasi -lc++ -lc++abi -lc -O3 --sysroot=/wasi-sysroot -Wl,--demangle -Wl,--allow-undefined -o .libs/libcre2.so \
    -Wl,--export=malloc \
    -Wl,--export=free \
    -Wl,--export=cre2_new \
    -Wl,--export=cre2_delete \
    -Wl,--export=cre2_opt_new \
    -Wl,--export=cre2_opt_delete \
    -Wl,--export=cre2_opt_set_max_mem \
    -Wl,--export=cre2_opt_set_longest_match \
    -Wl,--export=cre2_opt_set_posix_syntax \
    -Wl,--export=cre2_error_code \
    -Wl,--export=cre2_error_arg \
    -Wl,--export=cre2_num_capturing_groups \
    -Wl,--export=cre2_match \
    -Wl,--export=cre2_partial_match_re \
    -Wl,--export=cre2_find_and_consume_re \
    -Wl,--export=cre2_named_groups_iter_new \
    -Wl,--export=cre2_named_groups_iter_next \
    -Wl,--export=cre2_named_groups_iter_delete \
    -Wl,--export=cre2_replace_re \
    -Wl,--export=cre2_global_replace_re

CMD ["bash", "-c", "cp /re2/obj/libre2.a /cre2/.libs/libcre2.so /cre2/.libs/libcre2.a /out/"]
