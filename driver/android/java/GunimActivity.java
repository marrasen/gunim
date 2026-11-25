package gunim.android;

import android.app.Activity;
import android.os.Bundle;
import android.view.View;

/**
 * GunimActivity runs a gunim program. It loads the program, built as
 * libgunim.so, shows one GunimView, and starts the program's main
 * function, unless a component of the program's own has booted it.
 */
public class GunimActivity extends Activity {
	// waits says the activity came once the program was done with its
	// windows, and waits for its main to run anew, in a new process. It
	// tells of its intent there.
	private boolean waits;
	private static final String WAITS = "gunim.waits";

	private static final String CREATING = "gunim.creating";

	@Override
	@SuppressWarnings({"deprecation", "unchecked"})
	protected void onCreate(Bundle state) {
		super.onCreate(state);
		Native.load(this);
		if (state != null && state.getSerializable(CREATING) != null) {
			// The process that asked may have ended since.
			Native.strays.putAll((java.util.HashMap<Integer, Long>) state.getSerializable(CREATING));
		}
		waits = Native.waits();
		Native.activity = this;
		GunimView v = view = new GunimView(this);
		Native.view = v;
		Native.keepGestures(v);
		setContentView(v);
		edgeToEdge(v);
		applyAwake();
		if (android.os.Build.VERSION.SDK_INT >= 30) {
			// The view keeps its size as the keyboard opens; the driver
			// slides the drawing up instead, following the keyboard.
			getWindow().setSoftInputMode(android.view.WindowManager.LayoutParams.SOFT_INPUT_ADJUST_NOTHING
				| android.view.WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN);
			v.watchKeyboard(getWindow().getDecorView());
		}
		if (android.os.Build.VERSION.SDK_INT >= 33) {
			watchBack();
		}
		v.requestFocus();
		Native.boot(this);
		showBuild();
		// An activity Android brings back has told of its intent already.
		if (!waits && (state == null || state.getBoolean(WAITS))) {
			GunimOpen.opened(this, getIntent());
		}
	}

	@Override
	protected void onSaveInstanceState(Bundle out) {
		super.onSaveInstanceState(out);
		out.putBoolean(WAITS, waits);
		if (!Native.creating.isEmpty()) {
			out.putSerializable(CREATING, new java.util.HashMap<>(Native.creating));
		}
	}

	// With -opens the activity runs once, so a file opened while the
	// program runs comes here.
	@Override
	protected void onNewIntent(android.content.Intent intent) {
		super.onNewIntent(intent);
		setIntent(intent);
		if (!waits) {
			GunimOpen.opened(this, intent);
		}
	}

	/**
	 * applyAwake sets the window's flag to keep the screen on to what the
	 * program last asked, on the UI thread.
	 */
	void applyAwake() {
		int flag = android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON;
		if (Native.awake) {
			getWindow().addFlags(flag);
		} else {
			getWindow().clearFlags(flag);
		}
	}

	// view is this activity's own, as Native.view is the newest's.
	private GunimView view;

	// secured says that this activity is secure as gunim made it, and
	// flagged and filtered what it was before by the program's own doing.
	private boolean secured, flagged, filtered;

	// INHERITS is the oldest Android known to keep the surface of a view
	// out of recordings as it does its window. On one before it the surface
	// is made again, as it is made one way or the other for good.
	private static final int INHERITS = 34;

	/**
	 * applySecure sets the window to what the program last asked: kept out
	 * of screenshots, recordings, the picture in Recents and screens cast,
	 * with other applications' windows over it hidden, and deaf to touches
	 * through one. What the program set by itself stays as it is. It runs
	 * on the UI thread, and done once the screen shows the window so, with
	 * false where such windows may stay.
	 */
	void applySecure(java.util.function.Consumer<Boolean> done) {
		boolean on = Native.secure;
		if (on == secured) {
			done.accept(!on || overlaysGo);
			return;
		}
		secured = on;
		int flag = android.view.WindowManager.LayoutParams.FLAG_SECURE;
		GunimView v = view;
		if (on) {
			flagged = (getWindow().getAttributes().flags & flag) != 0;
			getWindow().addFlags(flag);
			if (v != null) {
				filtered = v.getFilterTouchesWhenObscured();
				v.setFilterTouchesWhenObscured(true);
			}
		} else {
			if (!flagged) {
				getWindow().clearFlags(flag);
			}
			if (v != null) {
				v.setFilterTouchesWhenObscured(filtered);
			}
		}
		if (android.os.Build.VERSION.SDK_INT >= 33) {
			setRecentsScreenshotEnabled(!on);
		}
		hideOverlays(on);
		Runnable ready = () -> done.accept(!on || overlaysGo);
		// The mark of the view's own is made with its surface, and stays with
		// that: a view that inherits its window's has none.
		boolean marked = android.os.Build.VERSION.SDK_INT < INHERITS;
		if (v != null && marked && GunimWatch.isShown(this)) {
			v.remake(on, () -> afterFrames(ready));
		} else {
			if (v != null && marked) {
				v.setSecure(on);
			}
			if (v != null && GunimWatch.isShown(this)) {
				afterFrames(ready);
			} else {
				// It is made, and drawn, as it shows.
				ready.run();
			}
		}
	}

	// overlaysGo says that other applications' windows over this one are
	// hidden while it is secure.
	private boolean overlaysGo;

	private void hideOverlays(boolean on) {
		overlaysGo = false;
		if (android.os.Build.VERSION.SDK_INT >= 31) {
			try {
				getWindow().setHideOverlayWindows(on);
				overlaysGo = true;
			} catch (SecurityException e) {
				// The program has no permission to.
			}
		}
	}

	// afterFrames runs done once the window has been laid out with what was
	// set, and drawn.
	private void afterFrames(Runnable done) {
		View v = getWindow().getDecorView();
		v.postOnAnimation(() -> v.postOnAnimation(() -> v.post(done)));
	}

	/**
	 * watchBack hands the back gesture and button to Go as the back key,
	 * which the driver takes as Escape. Android 13 and later tell an app
	 * of them through this callback rather than as a key, and Android 16
	 * only so, for an app built for it; GunimView takes the key on older
	 * Android.
	 */
	@android.annotation.TargetApi(33)
	private void watchBack() {
		getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
			android.window.OnBackInvokedDispatcher.PRIORITY_DEFAULT, () -> {
				Native.key(true, android.view.KeyEvent.KEYCODE_BACK, 0, 0, 0);
				Native.key(false, android.view.KeyEvent.KEYCODE_BACK, 0, 0, 0);
			});
	}

	/**
	 * edgeToEdge lays the view under the status bar, the navigation bar
	 * and the camera's cutout, with the bars clear, and tells Go how far
	 * in from each edge they reach, so a program draws its background to
	 * the screen's edges and keeps what it shows clear of them.
	 */
	@SuppressWarnings("deprecation")
	private void edgeToEdge(GunimView v) {
		android.view.Window w = getWindow();
		w.setStatusBarColor(android.graphics.Color.TRANSPARENT);
		w.setNavigationBarColor(android.graphics.Color.TRANSPARENT);
		if (android.os.Build.VERSION.SDK_INT >= 29) {
			w.setStatusBarContrastEnforced(false);
			w.setNavigationBarContrastEnforced(false);
		}
		if (android.os.Build.VERSION.SDK_INT >= 28) {
			android.view.WindowManager.LayoutParams lp = w.getAttributes();
			lp.layoutInDisplayCutoutMode = android.os.Build.VERSION.SDK_INT >= 30
				? android.view.WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS
				: android.view.WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
			w.setAttributes(lp);
		}
		if (android.os.Build.VERSION.SDK_INT >= 30) {
			w.setDecorFitsSystemWindows(false);
		} else {
			w.getDecorView().setSystemUiVisibility(android.view.View.SYSTEM_UI_FLAG_LAYOUT_STABLE
				| android.view.View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
				| android.view.View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION);
		}
		v.watchInsets(w.getDecorView());
	}

	/**
	 * showBuild says which build runs, in a debug build: gunimapk names
	 * each for its commit and the time it was built.
	 */
	private void showBuild() {
		if ((getApplicationInfo().flags & android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE) == 0) {
			return;
		}
		try {
			String version = getPackageManager().getPackageInfo(getPackageName(), 0).versionName;
			android.widget.Toast.makeText(this, getApplicationInfo().loadLabel(getPackageManager()) + " " + version,
				android.widget.Toast.LENGTH_LONG).show();
		} catch (android.content.pm.PackageManager.NameNotFoundException e) {
			// The build has no name to show.
		}
	}

	// The program hears when the activity can no longer be seen, gone to
	// the background, and when it can again, to stop and start what only
	// matters while it is seen, as a game's music. Where Android replaces
	// the activity, as at start, the new one starts before the old one
	// stops, so only the activity the program runs in now speaks for it.
	@Override
	protected void onStart() {
		// Nothing has shown yet, and the program has set what it will in an
		// onCreate of its own.
		applySecure((alone) -> {
		});
		super.onStart();
		if (Native.activity == this) {
			Native.shown(true);
		}
	}

	@Override
	protected void onStop() {
		// Its surface is made as it shows again.
		view.settled();
		if (Native.activity == this) {
			Native.shown(false);
		}
		super.onStop();
	}

	@Override
	protected void onDestroy() {
		if (Native.activity == this) {
			Native.activity = null;
			Native.view = null;
		}
		super.onDestroy();
	}

	// The user answered a permission's prompt: granted where every
	// permission asked was.
	@Override
	public void onRequestPermissionsResult(int code, String[] permissions, int[] results) {
		boolean granted = results.length > 0;
		for (int r : results) {
			granted &= r == android.content.pm.PackageManager.PERMISSION_GRANTED;
		}
		Native.answered(code, granted);
	}

	// The system's document picker closed, with the files chosen or none,
	// or its chooser of folders, with the folder chosen or none.
	@Override
	protected void onActivityResult(int code, int result, android.content.Intent data) {
		Long asked = Native.creating.remove(code);
		if (asked != null) {
			Native.strays.remove(code);
			Native.picked(code, asked, result == RESULT_OK && data != null ? data.getData() : null);
			return;
		}
		Long stray = Native.strays.remove(code);
		if (stray != null) {
			android.net.Uri made = result == RESULT_OK && data != null ? data.getData() : null;
			if (made != null) {
				new Thread(() -> GunimSave.stray(made, stray)).start();
			}
			return;
		}
		Boolean streams = Native.choosingFiles.remove(code);
		if (streams != null) {
			GunimOpen.chosen(this, code, result == RESULT_OK ? data : null, streams);
			return;
		}
		String path = null;
		if (result == RESULT_OK && data != null) {
			path = Native.pathOf(data.getData());
		}
		Native.chosen(code, path);
	}
}
