# Сборка Linux-драйвера под Steam Runtime «sniper» (glibc 2.31): clang + sysroot Debian 11 (fetch_sysroot.py).
# libstdc++ и libgcc берутся из sysroot (GCC 10) и линкуются статически (CMakeLists.txt).
set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR x86_64)
set(CMAKE_SYSROOT "${CMAKE_CURRENT_LIST_DIR}/../third_party/sysroot-bullseye")
set(CMAKE_CXX_COMPILER clang++)
set(CMAKE_CXX_COMPILER_TARGET x86_64-linux-gnu)
set(CMAKE_SHARED_LINKER_FLAGS_INIT "-fuse-ld=lld")
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
