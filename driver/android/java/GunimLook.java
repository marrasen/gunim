package gunim.android;

import android.content.ComponentCallbacks;
import android.content.Context;
import android.content.res.Configuration;
import android.database.ContentObserver;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;

/**
 * GunimLook tells Go how the user has the system look: dark or light, the
 * size of text, and whether things move.
 */
@SuppressWarnings("deprecation")
final class GunimLook extends ContentObserver implements ComponentCallbacks {
	private static GunimLook watching;

	private GunimLook() {
		super(new Handler(Looper.getMainLooper()));
	}

	/** read returns the size of text, 1 as made, and 1 each for dark and for no motion. */
	static float[] read() {
		Context app = Native.app;
		if (app == null) {
			return null;
		}
		Configuration c = app.getResources().getConfiguration();
		boolean dark = (c.uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
		boolean still = Settings.Global.getFloat(app.getContentResolver(), Settings.Global.ANIMATOR_DURATION_SCALE, 1) == 0;
		return new float[] {c.fontScale, dark ? 1 : 0, still ? 1 : 0};
	}

	/** watch starts listening for a change. */
	static synchronized void watch() {
		if (watching != null || Native.app == null) {
			return;
		}
		watching = new GunimLook();
		Native.app.registerComponentCallbacks(watching);
		Native.app.getContentResolver().registerContentObserver(
			Settings.Global.getUriFor(Settings.Global.ANIMATOR_DURATION_SCALE), false, watching);
	}

	@Override
	public void onConfigurationChanged(Configuration c) {
		Native.look();
	}

	@Override
	public void onChange(boolean self) {
		Native.look();
	}

	@Override
	public void onLowMemory() {
	}
}
