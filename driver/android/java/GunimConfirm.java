package gunim.android;

import android.app.KeyguardManager;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.hardware.biometrics.BiometricManager;
import android.hardware.biometrics.BiometricPrompt;
import android.os.Build;
import android.os.CancellationSignal;
import android.os.Handler;
import android.os.Looper;

/**
 * GunimConfirm asks, with Android's own prompt, whether it is the user who
 * holds the phone, and tells Go the answer. All of it runs on the UI thread,
 * and there is one at a time.
 */
final class GunimConfirm {
	// The answers, as Go has them. Only a word of Android's own is YES.
	private static final int YES = 0, NO = 1, NO_LOCK = 2, FAILED = 3, REFUSED = 4;

	private static final Handler ui = new Handler(Looper.getMainLooper());

	// waiting is the prompt that waits for the activity to be in front, and
	// up the one that shows, or null.
	private static GunimConfirm waiting, up;

	// front is the activity in front, or null.
	private static GunimActivity front;

	private final long run;
	private final String title, text;
	private final boolean strong;

	// signal takes the prompt away, from Android 10, and code is what the
	// activity of the prompt before that answers to.
	private CancellationSignal signal;
	private int code;

	private GunimConfirm(long run, String title, String text, boolean strong) {
		this.run = run;
		this.title = title;
		this.text = text;
		this.strong = strong;
	}

	/** hasLock reports whether the phone has a screen lock to confirm with. */
	static boolean hasLock(Context c) {
		return c.getSystemService(KeyguardManager.class).isDeviceSecure();
	}

	/** ask shows the prompt of run, once the activity is in front. */
	static void ask(long run, String title, String text, boolean strong) {
		ui.post(() -> {
			waiting = new GunimConfirm(run, title, text, strong);
			show();
		});
	}

	/** cancel takes the prompt of run away. */
	static void cancel(long run) {
		ui.post(() -> {
			if (waiting != null && waiting.run == run) {
				waiting = null;
			}
			GunimConfirm c = up;
			if (c == null || c.run != run) {
				return;
			}
			up = null;
			if (c.signal != null) {
				c.signal.cancel();
			} else if (Native.activity != null) {
				Native.activity.finishActivity(c.code);
			}
		});
	}

	/** resumed says that a is in front, and paused that it is no more. */
	static void resumed(GunimActivity a) {
		front = a;
		show();
	}

	static void paused(GunimActivity a) {
		if (front == a) {
			front = null;
		}
	}

	private static void show() {
		GunimConfirm c = waiting;
		if (c == null || front == null) {
			return;
		}
		waiting = null;
		up = c;
		try {
			c.start(front);
		} catch (SecurityException e) {
			c.answer(REFUSED);
		} catch (RuntimeException e) {
			c.answer(FAILED);
		}
	}

	private void start(GunimActivity a) {
		if (!hasLock(a)) {
			answer(NO_LOCK);
			return;
		}
		CharSequence title = this.title.isEmpty() ? a.getApplicationInfo().loadLabel(a.getPackageManager()) : this.title;
		if (Build.VERSION.SDK_INT >= 29) {
			startPrompt(a, title);
			return;
		}
		@SuppressWarnings("deprecation")
		Intent i = a.getSystemService(KeyguardManager.class).createConfirmDeviceCredentialIntent(title, text);
		if (i == null) {
			answer(NO_LOCK);
			return;
		}
		// Go's codes lie under these.
		code = CODES | (int) (run & 0xffff);
		try {
			a.startActivityForResult(i, code);
		} catch (ActivityNotFoundException e) {
			answer(FAILED);
		}
	}

	private static final int CODES = 0x40000000;

	@android.annotation.TargetApi(29)
	@SuppressWarnings("deprecation")
	private void startPrompt(GunimActivity a, CharSequence title) {
		BiometricPrompt.Builder b = new BiometricPrompt.Builder(a).setTitle(title);
		if (!text.isEmpty()) {
			b.setDescription(text);
		}
		if (Build.VERSION.SDK_INT >= 30) {
			b.setAllowedAuthenticators(BiometricManager.Authenticators.DEVICE_CREDENTIAL
				| (strong ? BiometricManager.Authenticators.BIOMETRIC_STRONG : BiometricManager.Authenticators.BIOMETRIC_WEAK));
		} else {
			b.setDeviceCredentialAllowed(true);
		}
		signal = new CancellationSignal();
		b.build().authenticate(signal, a.getMainExecutor(), new BiometricPrompt.AuthenticationCallback() {
			@Override
			public void onAuthenticationSucceeded(BiometricPrompt.AuthenticationResult r) {
				answer(YES);
			}

			@Override
			public void onAuthenticationError(int error, CharSequence why) {
				// A finger not known is no error, and the prompt stays for it.
				answer(error == BiometricPrompt.BIOMETRIC_ERROR_NO_DEVICE_CREDENTIAL ? NO_LOCK : NO);
			}
		});
	}

	/** result takes the answer of the prompt's activity, and reports whether code is one's. */
	static boolean result(int code, boolean ok) {
		if ((code & ~0xffff) != CODES) {
			return false;
		}
		GunimConfirm c = up;
		if (c != null && c.signal == null && c.code == code) {
			c.answer(ok ? YES : NO);
		}
		return true;
	}

	// answer tells Go, where the prompt is the one that is up.
	private void answer(int a) {
		if (up != this) {
			return;
		}
		up = null;
		Native.confirmed(run, a);
	}
}
