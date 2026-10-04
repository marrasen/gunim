package gunim.android;

import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.os.Handler;
import android.os.Looper;
import android.view.Surface;

/**
 * Native is the bridge to the Go half of gunim's Android driver. Its
 * native methods hand Go what happens on the UI thread; its static
 * methods are what Go calls, from threads of its own.
 */
public final class Native {
	static GunimActivity activity;
	static GunimView view;
	// app is the application's context, which outlives any activity.
	static Context app;

	private static final Handler ui = new Handler(Looper.getMainLooper());

	static native void start();
	static native void surfaceChanged(Surface surface, int width, int height);
	static native void surfaceDestroyed();
	static native void metrics(float density, float refreshRate);
	static native void touch(int action, float x, float y, long time);
	static native void pinch(int action, float x0, float y0, float x1, float y1);
	static native void key(boolean down, int code, int meta, int ch, int repeat);
	static native void focus(boolean focused);
	static native void shown(boolean shown);
	static native void keyboard(int px);
	static native void insets(int top, int right, int bottom, int left);
	static native void edit(String with, int replaceStart, int replaceEnd, int selAnchor, int selCaret,
		int compStart, int compEnd, long seq);
	static native void text(String s);
	static native void composing(String s, int selStart, int selEnd);

	// Called from Go.

	static void keepRunning(boolean on, String title, String text) {
		ui.post(() -> {
			if (app == null) {
				return;
			}
			if (!on) {
				app.stopService(new android.content.Intent(app, GunimService.class));
				return;
			}
			if (GunimService.update(title, text)) {
				return;
			}
			GunimActivity a = activity;
			if (a != null) {
				a.askToNotify();
			}
			android.content.Intent i = new android.content.Intent(app, GunimService.class);
			i.putExtra("title", title);
			i.putExtra("text", text);
			app.startForegroundService(i);
		});
	}

	static void showKeyboard(boolean show) {
		ui.post(() -> {
			if (view != null) {
				view.showKeyboard(show);
			}
		});
	}

	static void caret(int x0, int y0, int x1, int y1) {
		ui.post(() -> {
			if (view != null) {
				view.setCaret(x0, y0, x1, y1);
			}
		});
	}

	static void textState(char[] text, int start, int selA, int selB, int compA, int compB,
		boolean multiline, boolean secret, long seq) {
		String s = new String(text);
		ui.post(() -> {
			if (view != null) {
				view.input.setState(s, start, selA, selB, compA, compB, multiline, secret, seq);
			}
		});
	}

	static void clearTextState() {
		ui.post(() -> {
			if (view != null) {
				view.input.clearState();
			}
		});
	}

	static String getClipboard() {
		GunimActivity a = activity;
		if (a == null) {
			return null;
		}
		ClipboardManager cm = (ClipboardManager) a.getSystemService(Context.CLIPBOARD_SERVICE);
		ClipData clip = cm.getPrimaryClip();
		if (clip == null || clip.getItemCount() == 0) {
			return null;
		}
		CharSequence s = clip.getItemAt(0).coerceToText(a);
		return s == null ? null : s.toString();
	}

	static void setClipboard(String s) {
		GunimActivity a = activity;
		if (a == null) {
			return;
		}
		ClipboardManager cm = (ClipboardManager) a.getSystemService(Context.CLIPBOARD_SERVICE);
		cm.setPrimaryClip(ClipData.newPlainText("text", s));
	}

	static void buzz() {
		ui.post(() -> {
			if (view != null) {
				view.performHapticFeedback(android.view.HapticFeedbackConstants.LONG_PRESS);
			}
		});
	}

	static void finish() {
		ui.post(() -> {
			if (activity != null) {
				activity.finish();
			}
		});
	}
}
