cmake_minimum_required(VERSION 3.22)
project(picotrackers_headset CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Качает fetch_deps.sh: loader из PICO OpenXR SDK 2.2.0 (его ALVR использует для Pico 4 / 4 Pro)
# и заголовки Khronos OpenXR-SDK (в заголовках PICO SDK нет XR_BD_body_tracking).
set(PICO_OPENXR_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../third_party/pico_openxr" CACHE PATH "PICO OpenXR SDK")
set(OPENXR_HEADERS_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../third_party/openxr/include" CACHE PATH "OpenXR headers")

add_library(openxr_loader SHARED IMPORTED)
set_target_properties(openxr_loader PROPERTIES
    IMPORTED_LOCATION "${PICO_OPENXR_DIR}/libs/android.arm64-v8a/libopenxr_loader.so"
    INTERFACE_INCLUDE_DIRECTORIES "${OPENXR_HEADERS_DIR}")

add_library(picotrackers SHARED tracking.cpp)
target_include_directories(picotrackers PRIVATE "${CMAKE_CURRENT_SOURCE_DIR}/../../common")
target_compile_options(picotrackers PRIVATE -Wall -Wextra -Wno-unused-parameter -Wno-missing-field-initializers)
target_link_options(picotrackers PRIVATE -s)
target_link_libraries(picotrackers PRIVATE openxr_loader android log)
