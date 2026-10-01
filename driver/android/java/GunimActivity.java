package gunim.android;

import android.app.Activity;
import android.os.Bundle;
import android.system.ErrnoException;
import android.system.Os;

/**
 * GunimActivity runs a gunim program. It loads the program, built as
 * libgunim.so, shows one GunimView, and starts the program's main
 * function the first time.
 */
public class GunimActivity extends Activity {
	@Override
	protected void onCreate(Bundle state) {
		super.onCreate(state);
		// Go looks for its home and caches where a desktop keeps them.
		try {
			Os.setenv("HOME", getFilesDir().getPath(), true);
			Os.setenv("TMPDIR", getCacheDir().getPath(), true);
			Os.setenv("XDG_CACHE_HOME", getCacheDir().getPath(), true);
			Os.setenv("XDG_CONFIG_HOME", getFilesDir().getPath(), true);
		} catch (ErrnoException e) {
			// Go falls back to its defaults.
		}
		System.loadLibrary("gunim");
		Native.activity = this;
		GunimView v = new GunimView(this);
		Native.view = v;
		setContentView(v);
		v.requestFocus();
		Native.start();
	}

	@Override
	protected void onDestroy() {
		if (Native.activity == this) {
			Native.activity = null;
			Native.view = null;
		}
		super.onDestroy();
	}
}
