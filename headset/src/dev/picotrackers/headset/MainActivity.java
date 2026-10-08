package dev.picotrackers.headset;

import android.app.Activity;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Typeface;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

/**
 * Окно статуса. Открытие окна запускает сервис в авторежиме (ждёт SteamVR); ручная передача
 * на указанный IP — для отладки без SteamVR.
 */
public class MainActivity extends Activity {
    private final Handler handler = new Handler(Looper.getMainLooper());
    private TextView status;
    private EditText host;

    private final Runnable refresh = new Runnable() {
        @Override
        public void run() {
            status.setText(TrackingService.state() + "\n\n" + Native.status());
            handler.postDelayed(this, 500);
        }
    };

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        SharedPreferences prefs = getPreferences(MODE_PRIVATE);
        startForegroundService(new Intent(this, TrackingService.class));

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(32, 32, 32, 32);

        TextView title = new TextView(this);
        title.setText("PICO Trackers");
        title.setTextSize(22);
        root.addView(title);

        TextView hint = new TextView(this);
        hint.setText("Обычно ничего нажимать не нужно: когда на ПК запускается SteamVR с драйвером "
                + "picotrackers, трекинг включается сам и выключается, когда SteamVR закрывается.\n\n"
                + "Для отладки без SteamVR — передавать на IP (UDP-порт " + TrackingService.DEFAULT_PORT + "):");
        root.addView(hint);

        host = new EditText(this);
        host.setSingleLine(true);
        host.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        host.setText(prefs.getString("host", "255.255.255.255"));
        root.addView(host);

        LinearLayout buttons = new LinearLayout(this);
        Button manual = new Button(this);
        manual.setText("Передавать вручную");
        manual.setOnClickListener(v -> {
            String h = host.getText().toString().trim();
            prefs.edit().putString("host", h).apply();
            startForegroundService(
                    new Intent(this, TrackingService.class).putExtra(TrackingService.EXTRA_HOST, h));
        });
        buttons.addView(manual);
        Button auto = new Button(this);
        auto.setText("Авто");
        auto.setOnClickListener(v -> startForegroundService(new Intent(this, TrackingService.class)));
        buttons.addView(auto);
        Button off = new Button(this);
        off.setText("Выключить");
        off.setOnClickListener(v -> stopService(new Intent(this, TrackingService.class)));
        buttons.addView(off);
        root.addView(buttons);

        status = new TextView(this);
        status.setTypeface(Typeface.MONOSPACE);
        status.setTextSize(14);
        root.addView(status);

        ScrollView scroll = new ScrollView(this);
        scroll.addView(root);
        setContentView(scroll);
    }

    @Override
    protected void onResume() {
        super.onResume();
        handler.post(refresh);
    }

    @Override
    protected void onPause() {
        handler.removeCallbacks(refresh);
        super.onPause();
    }
}
