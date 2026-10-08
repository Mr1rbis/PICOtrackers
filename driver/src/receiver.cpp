#include "receiver.h"

#ifdef _WIN32
#include <ws2tcpip.h>
#else
#include <arpa/inet.h>
#include <ifaddrs.h>
#include <net/if.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>
#endif

#include <cerrno>
#include <cstring>
#include <vector>

#include "log.h"

namespace picotrackers {

namespace {

constexpr auto kAnnounceInterval = std::chrono::seconds(1);

#ifdef _WIN32
void close_socket(Socket s) { closesocket(s); }
std::string socket_error() { return "ошибка " + std::to_string(WSAGetLastError()); }
#else
void close_socket(Socket s) { close(s); }
std::string socket_error() { return strerror(errno); }
#endif

void send_to(Socket fd, const void *data, size_t size, in_addr address, uint16_t port) {
    sockaddr_in to{};
    to.sin_family = AF_INET;
    to.sin_addr = address;
    to.sin_port = htons(port);
    sendto(fd, static_cast<const char *>(data), static_cast<int>(size), 0, reinterpret_cast<sockaddr *>(&to),
           sizeof(to));
}

// Широковещательные адреса: на Linux — всех IPv4-сетей, на Windows — общий 255.255.255.255.
std::vector<in_addr> broadcast_addresses() {
    std::vector<in_addr> result;
#ifdef _WIN32
    result.push_back(in_addr{});
    result.back().s_addr = htonl(INADDR_BROADCAST);
#else
    ifaddrs *interfaces = nullptr;
    if (getifaddrs(&interfaces) == 0) {
        for (ifaddrs *i = interfaces; i; i = i->ifa_next) {
            if (!i->ifa_addr || i->ifa_addr->sa_family != AF_INET || !i->ifa_broadaddr) continue;
            if (!(i->ifa_flags & IFF_UP) || !(i->ifa_flags & IFF_BROADCAST) || (i->ifa_flags & IFF_LOOPBACK)) continue;
            result.push_back(reinterpret_cast<sockaddr_in *>(i->ifa_broadaddr)->sin_addr);
        }
        freeifaddrs(interfaces);
    }
#endif
    return result;
}

}  // namespace

bool Receiver::start(uint16_t port, const std::string &known_headset_ip) {
    port_ = port;
    headset_ip_ = known_headset_ip;
#ifdef _WIN32
    WSADATA wsa;
    WSAStartup(MAKEWORD(2, 2), &wsa);
#endif
#ifdef _WIN32
    fd_ = socket(AF_INET, SOCK_DGRAM, 0);
#else
    // CLOEXEC: иначе сокет наследуют процессы, которые запускает vrserver, и порт остаётся занят после SteamVR.
    fd_ = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
#endif
    if (fd_ == kNoSocket) {
        log("socket: %s", socket_error().c_str());
        return false;
    }
    // Таймаут приёма — чтобы поток замечал stop() и вовремя слал просьбы.
#ifdef _WIN32
    DWORD timeout = 200;
#else
    timeval timeout{0, 200 * 1000};
#endif
    setsockopt(fd_, SOL_SOCKET, SO_RCVTIMEO, reinterpret_cast<const char *>(&timeout), sizeof(timeout));
    int on = 1;
    setsockopt(fd_, SOL_SOCKET, SO_BROADCAST, reinterpret_cast<const char *>(&on), sizeof(on));

    sockaddr_in addr{};
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = htonl(INADDR_ANY);
    addr.sin_port = htons(port);
    if (bind(fd_, reinterpret_cast<sockaddr *>(&addr), sizeof(addr)) != 0) {
        log("bind UDP %u: %s", port, socket_error().c_str());
        close_socket(fd_);
        fd_ = kNoSocket;
        return false;
    }

    running_ = true;
    thread_ = std::thread(&Receiver::run, this);
    log("слушаю UDP-порт %u, шлем: %s", port, known_headset_ip.empty() ? "ищу по broadcast" : known_headset_ip.c_str());
    return true;
}

void Receiver::stop() {
    if (!running_.exchange(false)) return;
    if (thread_.joinable()) thread_.join();
    announce(kCommandBye);
    close_socket(fd_);
    fd_ = kNoSocket;
}

bool Receiver::latest(Snapshot &out) const {
    std::lock_guard<std::mutex> lock(mutex_);
    if (!has_latest_) return false;
    out = latest_;
    return true;
}

std::string Receiver::headset_ip() const {
    std::lock_guard<std::mutex> lock(mutex_);
    return headset_ip_;
}

bool Receiver::app_command(std::string &command, Clock::time_point &received) const {
    std::lock_guard<std::mutex> lock(mutex_);
    if (app_command_.empty()) return false;
    command = app_command_;
    received = app_command_received_;
    return true;
}

void Receiver::send_to_app(const std::string &message) {
    if (fd_ == kNoSocket) return;
    in_addr localhost{};
    localhost.s_addr = htonl(INADDR_LOOPBACK);
    send_to(fd_, message.data(), message.size(), localhost, kAppPort);
}

void Receiver::announce(ControlCommand command) {
    ControlPacket packet{};
    memcpy(packet.magic, kControlMagic, sizeof(packet.magic));
    packet.data_port = port_;
    packet.command = command;
    packet.rate_hz = rate_.load();

    for (in_addr address : broadcast_addresses()) send_to(fd_, &packet, sizeof(packet), address, kControlPort);

    in_addr headset{};
    std::string ip = headset_ip();
    if (!ip.empty() && inet_pton(AF_INET, ip.c_str(), &headset) == 1)
        send_to(fd_, &packet, sizeof(packet), headset, kControlPort);
}

void Receiver::run() {
    char buffer[2048];
    auto next_announce = Clock::now();
    bool requested = false;
    while (running_.load()) {
        auto now = Clock::now();
        bool request = request_data_.load();
        if (request && (now >= next_announce || announce_now_.exchange(false))) {
            next_announce = now + kAnnounceInterval;
            announce(kCommandHello);
        } else if (!request && requested) {
            announce(kCommandBye);
        }
        requested = request;

        sockaddr_in from{};
        socklen_t from_len = sizeof(from);
        int n = static_cast<int>(
            recvfrom(fd_, buffer, sizeof(buffer), 0, reinterpret_cast<sockaddr *>(&from), &from_len));
        if (n < 4) continue;

        if (memcmp(buffer, kAppMagic, sizeof(kAppMagic)) == 0 && from.sin_addr.s_addr == htonl(INADDR_LOOPBACK)) {
            std::lock_guard<std::mutex> lock(mutex_);
            app_command_.assign(buffer + 4, n - 4);
            app_command_received_ = Clock::now();
            continue;
        }

        Packet packet;
        if (n != static_cast<int>(sizeof(packet)) || memcmp(buffer, kMagic, sizeof(kMagic)) != 0) continue;
        memcpy(&packet, buffer, sizeof(packet));
        if (packet.joint_count > kJointCount) continue;

        char ip[INET_ADDRSTRLEN] = "";
        inet_ntop(AF_INET, &from.sin_addr, ip, sizeof(ip));
        std::lock_guard<std::mutex> lock(mutex_);
        latest_.packet = packet;
        latest_.received = Clock::now();
        has_latest_ = true;
        if (headset_ip_ != ip) headset_ip_ = ip;
        packets_++;
    }
}

}  // namespace picotrackers
