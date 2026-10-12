package dev.picotrackers.headset;

import android.content.Context;

/** Мост к libpicotrackers.so: поток трекинга живёт в нативном коде. */
final class Native {
    static {
        System.loadLibrary("openxr_loader");
        System.loadLibrary("picotrackers");
    }

    static native void start(Context context, String host, int port);

    static native void stop();

    /** Сколько снимков в секунду слать; применяется на лету. */
    static native void setRate(int rate);

    /** Идёт ли трекинг (поток мог завершиться сам после ошибки). */
    static native boolean running();

    static native String status();

    /** Язык строк состояния (status, picoTrackers). */
    static native void setRussian(boolean russian);

    /** Опросить трекеры PICO (libtrackingclient.pxr.so): false — библиотека недоступна. */
    static native boolean pollPicoTrackers();

    /** Трекеры PICO одной строкой: сколько подключено, заряд, калибровка. */
    static native String picoTrackers();

    /** Открыть калибровку трекеров через рантайм OpenXR: XrResult, < 0 — не получилось. */
    static native int startCalibration();

    /** Итог просьбы программы на ПК откалибровать трекеры — уходит ей в пакете состояния. */
    static native void setCalibrationResult(int request, int result);

    private Native() {}
}
