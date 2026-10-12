// Сеть драйвера в отдельном потоке:
//  * приём снимков скелета и состояния трекеров PICO со шлема (хранится только последнее);
//  * раз в секунду просьба «нужны данные» шлему — по широковещательным адресам и на известный IP шлема;
//  * приём команд программы на ПК и отправка ей состояния (127.0.0.1).
#pragma once

#include <atomic>
#include <chrono>
#include <cstdint>
#include <mutex>
#include <string>
#include <thread>

#include "protocol.h"

#ifdef _WIN32
#include <winsock2.h>
#endif

namespace picotrackers {

#ifdef _WIN32
using Socket = SOCKET;
constexpr Socket kNoSocket = INVALID_SOCKET;
#else
using Socket = int;
constexpr Socket kNoSocket = -1;
#endif

class Receiver {
public:
    using Clock = std::chrono::steady_clock;

    struct Snapshot {
        Packet packet{};
        Clock::time_point received;
    };
    struct Status {
        StatusPacket packet{};
        Clock::time_point received;
    };

    ~Receiver() { stop(); }

    // known_headset_ip — адрес шлема с прошлого запуска ("" — неизвестен, только broadcast).
    bool start(uint16_t port, const std::string &known_headset_ip);
    // Говорит шлему «пока» и останавливает поток.
    void stop();

    // false — пакетов ещё не было.
    bool latest(Snapshot &out) const;
    // Последнее состояние трекеров PICO; false — шлем его не присылал (приложение до 1.7).
    bool latest_status(Status &out) const;
    // Сколько пришло снимков с позами (без пакетов состояния).
    uint64_t packet_count() const { return packets_.load(); }
    // IP, с которого пришёл последний снимок ("" — ещё не было).
    std::string headset_ip() const;

    // Просить ли у шлема данные. При выключении шлем сразу получает «пока».
    void set_request_data(bool request) { request_data_ = request; }
    // Частота снимков, которую просим у шлема; при изменении просьба уходит сразу.
    void set_rate(uint16_t rate) {
        if (rate_.exchange(rate) != rate) announce_now_ = true;
    }
    // Попросить шлем открыть калибровку трекеров PICO (просьба уходит несколько раз — UDP может потерять пакет).
    void request_calibration(uint16_t request) {
        calibration_request_ = request;
        calibration_sends_ = 3;
    }

    // Последняя команда программы на ПК (без «PTA1 »). false — команд ещё не было.
    bool app_command(std::string &command, Clock::time_point &received) const;
    void send_to_app(const std::string &message);

private:
    void run();
    void announce(ControlCommand command, uint16_t request = 0);

    Socket fd_ = kNoSocket;
    uint16_t port_ = 0;
    std::thread thread_;
    std::atomic<bool> running_{false};
    std::atomic<bool> request_data_{true};
    std::atomic<uint16_t> rate_{100};
    std::atomic<bool> announce_now_{false};
    std::atomic<uint64_t> packets_{0};
    std::atomic<uint16_t> calibration_request_{0};
    std::atomic<int> calibration_sends_{0};

    mutable std::mutex mutex_;
    Snapshot latest_;
    bool has_latest_ = false;
    Status status_;
    bool has_status_ = false;
    std::string headset_ip_;
    std::string app_command_;
    Clock::time_point app_command_received_;
};

}  // namespace picotrackers
