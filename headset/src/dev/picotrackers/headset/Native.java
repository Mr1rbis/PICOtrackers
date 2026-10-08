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

    private Native() {}
}
