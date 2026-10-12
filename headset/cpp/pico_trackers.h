// Трекеры PICO (Motion Tracker): подключены ли, откалиброваны ли, заряд, открыть калибровку.
#pragma once

#include <cstdint>
#include <string>

#include <openxr/openxr.h>

#include "protocol.h"

namespace pico {

// Рантайм OpenXR текущего трекинга: через него открывается калибровка. XR_NULL_HANDLE — трекинг остановлен
// (вызывается до xrDestroyInstance, поэтому калибровка не попадёт на уничтоженный instance).
void set_runtime(XrInstance instance);

// Открыть калибровку трекеров через рантайм: результат XrResult (< 0 — не получилось, в том числе
// XR_ERROR_HANDLE_INVALID — трекинг не идёт и XR_ERROR_FUNCTION_UNSUPPORTED — у рантайма нет такой функции).
int32_t start_calibration();
// Чем закончилась просьба программы на ПК (уходит ей в StatusPacket).
void set_calibration_result(uint16_t request, int16_t result);

// Опрос libtrackingclient.pxr.so: подключённые трекеры, их заряд, калибровка. false — библиотека недоступна.
bool poll();
// Трекеры подключены, но не откалиброваны (по последнему опросу).
bool not_calibrated();
// Одна строка для окна приложения.
std::string describe(bool russian);

picotrackers::StatusPacket status_packet(int8_t body_status, int8_t body_error);

}  // namespace pico
