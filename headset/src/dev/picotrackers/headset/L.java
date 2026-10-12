package dev.picotrackers.headset;

import java.util.Locale;

/** Язык интерфейса: русский, если он язык системы шлема, иначе английский. */
final class L {
    static final boolean RU = "ru".equals(Locale.getDefault().getLanguage());

    static String t(String ru, String en) {
        return RU ? ru : en;
    }

    private L() {}
}
