package dev.picotrackers.headset;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.net.wifi.WifiManager;
import android.os.IBinder;
import android.os.SystemClock;
import android.util.Log;

import java.io.IOException;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetSocketAddress;
import java.net.SocketTimeoutException;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;

/**
 * Постоянный foreground-сервис. Ждёт от драйвера SteamVR просьбу «нужны данные» (UDP 7789,
 * common/protocol.h) и, пока просьбы идут, передаёт трекинг на адрес отправителя.
 * Запускается при включении шлема (BootReceiver), при открытии приложения или с ПК через adb.
 * Интент с EXTRA_HOST включает трекинг вручную (для отладки без SteamVR), интент без него — авторежим.
 * EXTRA_ENABLED — выбор пользователя (кнопка на шлеме или в программе на ПК): false — выключить сервис и не
 * запускать его самому (при включении шлема, открытии приложения), пока не включат снова (true).
 */
public class TrackingService extends Service {
    static final String TAG = "PicoTrackers";
    static final String EXTRA_HOST = "host";
    static final String EXTRA_PORT = "port";
    static final String EXTRA_ENABLED = "enabled";
    static final int DEFAULT_PORT = 7788;
    static final int CONTROL_PORT = 7789;
    private static final long HELLO_TIMEOUT_MS = 5000;
    private static final int COMMAND_HELLO = 1;
    private static final int COMMAND_BYE = 2;
    private static final String CHANNEL = "tracking";
    private static final String PREFS = "service";
    private static final String PREF_DISABLED = "disabled";

    private static volatile String state = "сервис не запущен";

    private WifiManager.WifiLock wifiLock;
    private WifiManager.MulticastLock multicastLock;
    private Thread listener;
    private volatile DatagramSocket socket;
    private volatile boolean running;

    // Текущий трекинг; меняется под synchronized(this).
    private String target;  // "ip:port", null — трекинг выключен
    private boolean manual;
    private long lastHello;

    /** Состояние для окна приложения. */
    static String state() {
        return state;
    }

    /** Пользователь выключил сервис: сам он не запускается, пока его не включат. */
    static boolean disabled(Context context) {
        return context.getSharedPreferences(PREFS, MODE_PRIVATE).getBoolean(PREF_DISABLED, false);
    }

    private void setDisabled(boolean disabled) {
        getSharedPreferences(PREFS, MODE_PRIVATE).edit().putBoolean(PREF_DISABLED, disabled).commit();
    }

    @Override
    @SuppressWarnings("deprecation")  // WIFI_MODE_FULL_LOW_LATENCY работает только на переднем плане
    public void onCreate() {
        super.onCreate();
        getSystemService(NotificationManager.class).createNotificationChannel(
                new NotificationChannel(CHANNEL, "Трекинг", NotificationManager.IMPORTANCE_LOW));
        startForeground(1, notification("Ждёт SteamVR"));
        state = "Ждёт SteamVR";

        // Без WifiLock Wi-Fi уходит в энергосбережение и пакеты приходят пачками;
        // без MulticastLock часть прошивок отбрасывает широковещательные просьбы драйвера.
        WifiManager wm = getApplicationContext().getSystemService(WifiManager.class);
        wifiLock = wm.createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, TAG);
        wifiLock.acquire();
        multicastLock = wm.createMulticastLock(TAG);
        multicastLock.acquire();

        running = true;
        listener = new Thread(this::listen, "picotrackers-listener");
        listener.start();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && intent.hasExtra(EXTRA_ENABLED)) {
            boolean enabled = intent.getBooleanExtra(EXTRA_ENABLED, true);
            setDisabled(!enabled);
            Log.i(TAG, "listener: service " + (enabled ? "enabled" : "disabled") + " by user");
        }
        if (disabled(this)) {
            stopSelf();  // в том числе перезапуск системой (START_STICKY, intent == null)
            return START_NOT_STICKY;
        }
        String host = intent != null ? intent.getStringExtra(EXTRA_HOST) : null;
        synchronized (this) {
            if (host != null && !host.isEmpty()) {
                manual = true;
                startTracking(host, intent.getIntExtra(EXTRA_PORT, DEFAULT_PORT), "вручную");
            } else if (manual) {
                manual = false;  // назад в авторежим: без просьб SteamVR трекинг остановится по таймауту
                lastHello = 0;
            }
        }
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        running = false;
        DatagramSocket s = socket;
        if (s != null) s.close();
        try {
            listener.join();
        } catch (InterruptedException ignored) {
        }
        synchronized (this) {
            stopTracking("сервис остановлен");
        }
        wifiLock.release();
        multicastLock.release();
        state = "Сервис остановлен";
        Log.i(TAG, "listener: service stopped");  // по этой строке программа на ПК видит, что сервис выключен
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private void listen() {
        try (DatagramSocket s = new DatagramSocket(null)) {
            socket = s;
            s.setReuseAddress(true);
            s.bind(new InetSocketAddress(CONTROL_PORT));
            s.setSoTimeout(1000);
            byte[] buffer = new byte[64];
            DatagramPacket packet = new DatagramPacket(buffer, buffer.length);
            Log.i(TAG, "listener: waiting for SteamVR on UDP " + CONTROL_PORT);
            while (running) {
                try {
                    packet.setLength(buffer.length);
                    s.receive(packet);
                    onControl(packet);
                } catch (SocketTimeoutException ignored) {
                }
                checkTimeout();
            }
        } catch (IOException e) {
            if (running) Log.e(TAG, "listener failed", e);
        }
    }

    private synchronized void onControl(DatagramPacket packet) {
        if (packet.getLength() < 8) return;
        ByteBuffer b = ByteBuffer.wrap(packet.getData(), packet.getOffset(), packet.getLength())
                .order(ByteOrder.LITTLE_ENDIAN);
        if (b.get() != 'P' || b.get() != 'T' || b.get() != 'C' || b.get() != '1') return;
        int port = b.getShort() & 0xffff;
        int command = b.getShort() & 0xffff;
        int rate = packet.getLength() >= 10 ? b.getShort() & 0xffff : 0;
        if (manual) return;
        if (command == COMMAND_HELLO && rate > 0) Native.setRate(rate);

        String from = packet.getAddress().getHostAddress() + ":" + port;
        if (command == COMMAND_HELLO) {
            lastHello = SystemClock.elapsedRealtime();
            if (!from.equals(target) || !Native.running())
                startTracking(packet.getAddress().getHostAddress(), port, "SteamVR просит данные");
        } else if (command == COMMAND_BYE && from.equals(target)) {
            stopTracking("SteamVR закрылся");
        }
    }

    private synchronized void checkTimeout() {
        if (!manual && target != null && SystemClock.elapsedRealtime() - lastHello > HELLO_TIMEOUT_MS)
            stopTracking("SteamVR не отвечает " + HELLO_TIMEOUT_MS / 1000 + " с");
    }

    private void startTracking(String host, int port, String reason) {
        Native.stop();
        Native.start(getApplicationContext(), host, port);
        target = host + ":" + port;
        Log.i(TAG, "listener: tracking -> " + target + " (" + reason + ")");
        setState("Передаёт трекинг → " + target + (manual ? " (вручную)" : ""));
    }

    private void stopTracking(String reason) {
        if (target == null) return;
        Native.stop();
        Log.i(TAG, "listener: tracking stopped (" + reason + ")");
        target = null;
        manual = false;
        setState("Ждёт SteamVR");
    }

    private void setState(String text) {
        state = text;
        getSystemService(NotificationManager.class).notify(1, notification(text));
    }

    private Notification notification(String text) {
        return new Notification.Builder(this, CHANNEL)
                .setContentTitle("PICO Trackers")
                .setContentText(text)
                .setSmallIcon(android.R.drawable.ic_menu_mylocation)
                .build();
    }
}
