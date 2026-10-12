cmake_minimum_required(VERSION 3.22)
project(picotrackers_driver CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Папка драйвера в формате SteamVR: driver.vrdrivermanifest, bin/linux64 и/или bin/win64, resources/.
# Linux- и Windows-сборки кладут свои библиотеки в одну и ту же папку (см. build.sh).
set(DRIVER_ROOT "${CMAKE_BINARY_DIR}/picotrackers" CACHE PATH "Папка драйвера SteamVR")
if(WIN32)
    set(DRIVER_BIN "${DRIVER_ROOT}/bin/win64")
else()
    set(DRIVER_BIN "${DRIVER_ROOT}/bin/linux64")
endif()

add_library(driver_picotrackers SHARED
    src/driver.cpp
    src/tracker_device.cpp
    src/receiver.cpp
    src/calibration.cpp)
target_include_directories(driver_picotrackers PRIVATE
    "${CMAKE_CURRENT_SOURCE_DIR}/third_party/openvr/headers"
    "${CMAKE_CURRENT_SOURCE_DIR}/../common")
set_target_properties(driver_picotrackers PROPERTIES
    PREFIX ""
    LIBRARY_OUTPUT_DIRECTORY "${DRIVER_BIN}"
    RUNTIME_OUTPUT_DIRECTORY "${DRIVER_BIN}"
    CXX_VISIBILITY_PRESET hidden)
target_compile_options(driver_picotrackers PRIVATE -Wall -Wextra -Wno-unused-parameter)
target_link_options(driver_picotrackers PRIVATE -s)

if(WIN32)
    # Всё статически: SteamVR не должен искать рядом libstdc++/libwinpthread.
    target_compile_definitions(driver_picotrackers PRIVATE WIN32_LEAN_AND_MEAN NOMINMAX)
    target_link_options(driver_picotrackers PRIVATE -static -static-libgcc -static-libstdc++)
    target_link_libraries(driver_picotrackers PRIVATE ws2_32)
else()
    # SteamVR грузит драйверы внутри контейнера Steam Runtime: свой libstdc++ не зависит от его версии.
    target_link_options(driver_picotrackers PRIVATE -static-libstdc++ -static-libgcc -Wl,--no-undefined)
    target_link_libraries(driver_picotrackers PRIVATE pthread)
endif()

# Файлы пакета (манифест, настройки, иконки) копируются при каждой сборке — и когда сам драйвер не менялся.
add_custom_target(driver_package ALL
    COMMAND "${CMAKE_COMMAND}" -E copy_directory "${CMAKE_CURRENT_SOURCE_DIR}/package" "${DRIVER_ROOT}")
