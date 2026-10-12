package dev.picotrackers.headset;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.wifi.WifiManager;
import android.os.IBinder;
import android.os.SystemClock;
import android.provider.Settings;
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
 * Просьба драйвера «откалибровать трекеры» (команда 3) открывает калибровку трекеров PICO. Раз в 5 с сервис
 * опрашивает трекеры PICO (подключение, заряд, калибровка — pico_trackers.cpp), а поток трекинга отправляет это на ПК.
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
    private static final int COMMAND_CALIBRATE = 3;
    /** Приложение PICO «Motion Tracker»: в нём калибруют трекеры. */
    static final String CALIBRATION_PACKAGE = "com.pvr.swift";
    private static final String STEAM_LINK_PACKAGE = "com.valvesoftware.steamlinkvr";
    private static final long TRACKERS_POLL_MS = 5000;
    // Опрос libtrackingclient.pxr.so застрахован: перед первым вызовом ставим метку, после — снимаем.
    // Осталась после перезапуска — процесс упал на вызове; для этой версии приложения опрос выключается.
    private static final String PREF_PXR_PROBING = "pxr_probing";
    private static final String PREF_PXR_CRASHED = "pxr_crashed_version";
    private static final String CHANNEL = "tracking";
    private static final String PREFS = "service";
    private static final String PREF_DISABLED = "disabled";

    private static volatile String state = L.t("Сервис не запущен", "Service is not running");

    private WifiManager.WifiLock wifiLock;
    private WifiManager.MulticastLock multicastLock;
    private Thread listener;
    private Thread trackersPoller;
    private volatile DatagramSocket socket;
    private volatile boolean running;

    // Текущий трекинг; меняется под synchronized(this).
    private String target;  // "ip:port", null — трекинг выключен
    private boolean manual;
    private long lastHello;
    private int lastCalibrationRequest = -1;

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
        Native.setRussian(L.RU);
        getSystemService(NotificationManager.class).createNotificationChannel(
                new NotificationChannel(CHANNEL, L.t("Трекинг", "Tracking"), NotificationManager.IMPORTANCE_LOW));
        startForeground(1, notification(waitingText()));
        state = waitingText();

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
        trackersPoller = new Thread(this::pollTrackers, "picotrackers-trackers");
        trackersPoller.start();
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
                startTracking(host, intent.getIntExtra(EXTRA_PORT, DEFAULT_PORT), "manual");
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
        trackersPoller.interrupt();
        try {
            listener.join();
        } catch (InterruptedException ignored) {
        }
        synchronized (this) {
            stopTracking("service stopped");
        }
        wifiLock.release();
        multicastLock.release();
        state = L.t("Сервис остановлен", "Service stopped");
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
        int request = packet.getLength() >= 12 ? b.getShort() & 0xffff : 0;
        if (command == COMMAND_CALIBRATE) {
            // Драйвер шлёт просьбу на broadcast и на адрес шлема — выполняем каждую один раз.
            if (request != lastCalibrationRequest) {
                lastCalibrationRequest = request;
                calibrate(request);
            }
            return;
        }
        if (manual) return;
        if (command == COMMAND_HELLO && rate > 0) Native.setRate(rate);

        String from = packet.getAddress().getHostAddress() + ":" + port;
        if (command == COMMAND_HELLO) {
            lastHello = SystemClock.elapsedRealtime();
            if (!from.equals(target) || !Native.running())
                startTracking(packet.getAddress().getHostAddress(), port, "SteamVR asks for data");
        } else if (command == COMMAND_BYE && from.equals(target)) {
            stopTracking("SteamVR closed");
        }
    }

    private synchronized void checkTimeout() {
        if (!manual && target != null && SystemClock.elapsedRealtime() - lastHello > HELLO_TIMEOUT_MS)
            stopTracking("no requests from SteamVR for " + HELLO_TIMEOUT_MS / 1000 + " s");
    }

    private void startTracking(String host, int port, String reason) {
        Native.stop();
        Native.start(getApplicationContext(), host, port);
        target = host + ":" + port;
        Log.i(TAG, "listener: tracking -> " + target + " (" + reason + ")");
        setState(L.t("Передаёт трекинг → ", "Sending tracking → ") + target + (manual ? L.t(" (вручную)", " (manual)") : ""));
    }

    private void stopTracking(String reason) {
        if (target == null) return;
        Native.stop();
        Log.i(TAG, "listener: tracking stopped (" + reason + ")");
        target = null;
        manual = false;
        setState(waitingText());
    }

    private static String waitingText() {
        return L.t("Ждёт SteamVR", "Waiting for SteamVR");
    }

    /**
     * Открыть калибровку трекеров PICO (просьба программы на ПК через драйвер): сначала через рантайм OpenXR,
     * иначе — запуском приложения калибровки. Итог уходит на ПК в пакете состояния:
     * 0 — открыл рантайм, 1 — запущено приложение, 2 — запущено, но фоновый запуск окон может быть
     * запрещён (нет разрешения «поверх других окон»), < 0 — не получилось.
     */
    private void calibrate(int request) {
        int runtime = Native.startCalibration();
        int result;
        if (runtime >= 0) {
            result = 0;
        } else if (launchCalibrationApp(this)) {
            result = Settings.canDrawOverlays(this) ? 1 : 2;
        } else {
            result = runtime;
        }
        Native.setCalibrationResult(request, result);
        Log.i(TAG, "calibration: request " + request + " -> " + result + " (runtime " + runtime + ")");
    }

    /** Запускает приложение калибровки трекеров; после неё оно вернёт пользователя в Steam Link, если тот есть. */
    static boolean launchCalibrationApp(Context context) {
        Intent intent = context.getPackageManager().getLaunchIntentForPackage(CALIBRATION_PACKAGE);
        if (intent == null) return false;
        boolean steamLink = context.getPackageManager().getLaunchIntentForPackage(STEAM_LINK_PACKAGE) != null;
        intent.putExtra("enter_flag", steamLink ? STEAM_LINK_PACKAGE : context.getPackageName());
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            context.startActivity(intent);
            return true;
        } catch (RuntimeException e) {
            Log.w(TAG, "calibration app: " + e);
            return false;
        }
    }

    /** Раз в 5 с: подключённые трекеры PICO, их заряд и калибровка (libtrackingclient.pxr.so). */
    private void pollTrackers() {
        SharedPreferences prefs = getSharedPreferences(PREFS, MODE_PRIVATE);
        int version = versionCode();
        if (prefs.getBoolean(PREF_PXR_PROBING, false)) {
            Log.w(TAG, "pxr: the process crashed while polling PICO trackers last time, polling disabled");
            prefs.edit().remove(PREF_PXR_PROBING).putInt(PREF_PXR_CRASHED, version).commit();
        }
        if (prefs.getInt(PREF_PXR_CRASHED, -1) == version) return;

        prefs.edit().putBoolean(PREF_PXR_PROBING, true).commit();
        boolean available = Native.pollPicoTrackers();
        prefs.edit().remove(PREF_PXR_PROBING).commit();
        if (!available) return;
        Log.i(TAG, "pxr: polling PICO trackers every " + TRACKERS_POLL_MS / 1000 + " s");
        while (running) {
            try {
                Thread.sleep(TRACKERS_POLL_MS);
            } catch (InterruptedException e) {
                return;
            }
            Native.pollPicoTrackers();
        }
    }

    @SuppressWarnings("deprecation")  // getLongVersionCode — с API 28, versionCode хватает
    private int versionCode() {
        try {
            return getPackageManager().getPackageInfo(getPackageName(), 0).versionCode;
        } catch (Exception e) {
            return 0;
        }
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
