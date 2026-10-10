package dev.picotrackers.headset;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

/** После включения шлема запускает сервис в режиме ожидания SteamVR — если пользователь его не выключил. */
public class BootReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        if (Intent.ACTION_BOOT_COMPLETED.equals(intent.getAction()) && !TrackingService.disabled(context))
            context.startForegroundService(new Intent(context, TrackingService.class));
    }
}
