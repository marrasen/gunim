package gunim.android;

import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Environment;
import android.os.Handler;
import android.os.Looper;
import android.provider.DocumentsContract;
import android.system.ErrnoException;
import android.system.Os;
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

	private static boolean loaded;

	/**
	 * boot starts the program where no activity has, from a service, a
	 * job or a receiver of the program's own, with any context of the
	 * program's. The program's main function runs on a thread of its own
	 * from here on, once for each process, and boot changes nothing once
	 * it runs. The program opens its windows as ever, and they show when
	 * an activity starts in the process. Where main has returned already,
	 * boot ends the process, and Android starts a job or an activity
	 * again in a new one; it sends no broadcast again. It waits for an
	 * activity of the program's own that shows to be left first.
	 *
	 * An Application class of the program's own runs in the process that
	 * serves shared files too, which has no program: boot does nothing
	 * there.
	 *
	 * Call it on the UI thread, where Android calls such a component,
	 * before the first native method of the program's own.
	 */
	public static void boot(Context c) {
		if (isFilesProcess()) {
			return;
		}
		load(c);
		late |= closing;
		if (over) {
			settle();
		}
		keepsQuiet = quiet(c);
		start(java.util.TimeZone.getDefault().getID(), keepsQuiet);
	}

	// keepsQuiet is what quiet reported.
	static boolean keepsQuiet;

	// quiet reports whether gunimapk's -quiet keeps what the program
	// writes out of the system's log.
	private static boolean quiet(Context c) {
		try {
			android.os.Bundle meta = c.getPackageManager().getApplicationInfo(c.getPackageName(),
				PackageManager.GET_META_DATA).metaData;
			return meta != null && meta.getBoolean("gunim.quiet");
		} catch (PackageManager.NameNotFoundException e) {
			return true;
		}
	}

	// isFilesProcess reports whether this is the process of GunimFiles
	// alone.
	private static boolean isFilesProcess() {
		if (Build.VERSION.SDK_INT >= 28) {
			return android.app.Application.getProcessName().endsWith(GunimFiles.PROCESS);
		}
		try (java.io.BufferedReader r = new java.io.BufferedReader(new java.io.FileReader("/proc/self/cmdline"))) {
			String line = r.readLine();
			return line != null && line.trim().endsWith(GunimFiles.PROCESS);
		} catch (java.io.IOException e) {
			return false;
		}
	}

	// closing says that the program is done with its windows, and over
	// that its main has returned. late says that something booted it in
	// between, and waits. The UI thread keeps all three.
	private static boolean closing, over, late;

	// anew ends the process, whose main has run, for Android to start
	// again in a new one what it just started here, or what comes next.
	private static void anew() {
		android.os.Process.killProcess(android.os.Process.myPid());
	}

	// waits reports whether what starts now comes too late for the main
	// that runs, and waits for the next.
	static boolean waits() {
		return closing;
	}

	// load loads the program, whose main waits for boot to run it. Where
	// main has run already it ends the process, unless that would take
	// another activity with it: see settle.
	static synchronized void load(Context c) {
		if (over && (starting || GunimWatch.isAlone(c) && !GunimServed.isOpen())) {
			anew();
		}
		if (loaded) {
			return;
		}
		app = c.getApplicationContext();
		// Go looks for its home and caches where a desktop keeps them.
		try {
			Os.setenv("HOME", app.getFilesDir().getPath(), true);
			Os.setenv("TMPDIR", app.getCacheDir().getPath(), true);
			Os.setenv("XDG_CACHE_HOME", app.getCacheDir().getPath(), true);
			Os.setenv("XDG_CONFIG_HOME", app.getFilesDir().getPath(), true);
		} catch (ErrnoException e) {
			// Go falls back to its defaults.
		}
		System.loadLibrary("gunim");
		loaded = true;
	}

	static native void start(String zone, boolean quiet);
	static native void go(long h, Context context);
	static native void surfaceChanged(Surface surface, int width, int height);
	static native void surfaceDestroyed(Surface surface);
	static native void metrics(float density, float refreshRate);
	static native boolean touch(int action, int id, float x, float y, long time);
	static native void pinch(int action, float x0, float y0, float x1, float y1);
	static native void finger(int action, int id, float x, float y);
	static native void key(boolean down, int code, int meta, int ch, int repeat);
	static native void focus(boolean focused);
	static native void shown(boolean shown);
	static native void keyboard(int px);
	static native void insets(int top, int right, int bottom, int left);
	static native void media(int action, long position);
	static native void edit(String with, int replaceStart, int replaceEnd, int selAnchor, int selCaret,
		int compStart, int compEnd, long seq);
	static native void text(String s);
	static native void composing(String s, int selStart, int selEnd);
	static native void answered(int code, boolean granted);
	static native void chosen(int code, String path);
	static native void files(int code, int[] fds, long[] ids, String names, String opens);
	static native void streams(int code, int[] fds, long[] ids, long[] stated, long[] held, String names, String types,
		String opens);
	static native void saved(int code, String place, String name, boolean made);
	static native void servedUse(long id, int by);
	static native void servedEnd();
	static native int servedRead(long id, long offset, int size, byte[] data);
	static native void back();
	static native void look();
	static native void heading(float[] rotation, int screen, int accuracy, float error);

	// Called from Go.

	// run has Go run the function h names, with the thread's JNIEnv and
	// the application's context, and returns what it throws, or null.
	static String run(long h) {
		try {
			go(h, app);
			return null;
		} catch (Throwable t) {
			return said(t);
		}
	}

	// said returns what t says of itself, or the name of its class, as a
	// toString of the program's own may throw or return null.
	private static String said(Throwable t) {
		try {
			String s = t.toString();
			if (s != null) {
				return s;
			}
		} catch (Throwable thrown) {
			// The class's name says what it can.
		}
		return t.getClass().getName();
	}

	static void nowPlaying(boolean on, boolean playing, String title, String artist, String album,
		long length, long position, byte[] art) {
		GunimService.State st = on ? new GunimService.State(playing, title, artist, album, length, position, art) : null;
		ui.post(() -> GunimService.show(app, st));
	}

	static void showKeyboard(boolean show) {
		ui.post(() -> {
			if (view != null) {
				view.showKeyboard(show);
			}
		});
	}

	// excluded is the rects excludeGestures last set, kept on the UI
	// thread so a view that replaces the last, as a new activity's does,
	// keeps them too; see keepGestures.
	private static java.util.List<android.graphics.Rect> excluded = java.util.Collections.emptyList();

	// excludeGestures keeps the system's edge gestures, as back, out of
	// the rects in r, four pixel coordinates each, from Android 10.
	static void excludeGestures(int[] r) {
		if (Build.VERSION.SDK_INT < 29) {
			return;
		}
		java.util.ArrayList<android.graphics.Rect> rects = new java.util.ArrayList<>();
		for (int i = 0; i + 3 < r.length; i += 4) {
			rects.add(new android.graphics.Rect(r[i], r[i + 1], r[i + 2], r[i + 3]));
		}
		ui.post(() -> {
			excluded = rects;
			if (view != null) {
				keepGestures(view);
			}
		});
	}

	// keepGestures gives v the rects excludeGestures last set. It runs on
	// the UI thread, for each view that becomes the one Go draws in.
	static void keepGestures(GunimView v) {
		if (Build.VERSION.SDK_INT >= 29) {
			v.setSystemGestureExclusionRects(excluded);
		}
	}

	static void caret(int x0, int y0, int x1, int y1) {
		ui.post(() -> {
			if (view != null) {
				view.setCaret(x0, y0, x1, y1);
			}
		});
	}

	static void textState(char[] text, int start, int selA, int selB, int compA, int compB,
		boolean multiline, boolean secret, boolean raw, long seq) {
		String s = new String(text);
		ui.post(() -> {
			if (view != null) {
				view.input.setState(s, start, selA, selB, compA, compB, multiline, secret, raw, seq);
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
		cm.setPrimaryClip(own(s, false));
	}

	// OURS is in the extras of the clips that this process puts there, to
	// know its own by: a word that names no program.
	private static final String FROM = "gunim.from", OURS = java.util.UUID.randomUUID().toString();

	// own returns s as a clip of this program's, marked sensitive or not.
	private static ClipData own(String s, boolean sensitive) {
		ClipData clip = ClipData.newPlainText("text", s);
		android.os.PersistableBundle extras = new android.os.PersistableBundle();
		extras.putString(FROM, OURS);
		if (sensitive) {
			// ClipDescription.EXTRA_IS_SENSITIVE, which keyboards read on
			// Android before 13 too.
			extras.putBoolean("android.content.extra.IS_SENSITIVE", true);
		}
		clip.getDescription().setExtras(extras);
		return clip;
	}

	private static void emptyClipboard(ClipboardManager cm) {
		if (Build.VERSION.SDK_INT >= 28) {
			cm.clearPrimaryClip();
		} else {
			cm.setPrimaryClip(ClipData.newPlainText("", ""));
		}
	}

	// setClipboardSensitive puts s on the clipboard marked sensitive, or
	// empties it for "". It returns false where it could not.
	static boolean setClipboardSensitive(String s) {
		try {
			ClipboardManager cm = (ClipboardManager) app.getSystemService(Context.CLIPBOARD_SERVICE);
			if (s.isEmpty()) {
				emptyClipboard(cm);
			} else {
				cm.setPrimaryClip(own(s, true));
			}
			return true;
		} catch (RuntimeException e) {
			return false;
		}
	}

	// clearClipboardIf empties the clipboard where it holds s from this
	// process: 1 where it did, 0 where it holds something else, which it
	// reads nothing of where that is another program's, and 2 where Android
	// does not tell.
	static int clearClipboardIf(String s) {
		try {
			ClipboardManager cm = (ClipboardManager) app.getSystemService(Context.CLIPBOARD_SERVICE);
			android.content.ClipDescription about = cm.getPrimaryClipDescription();
			if (about == null) {
				// It is empty, or Android tells the program in front alone.
				GunimView v = view;
				return v != null && v.hasWindowFocus() ? 0 : 2;
			}
			if (about.getExtras() == null || !OURS.equals(about.getExtras().getString(FROM))) {
				return 0;
			}
			ClipData clip = cm.getPrimaryClip();
			if (clip == null || clip.getItemCount() != 1 || !s.contentEquals(String.valueOf(clip.getItemAt(0).getText()))) {
				return 0;
			}
			emptyClipboard(cm);
			return 1;
		} catch (RuntimeException e) {
			return 2;
		}
	}

	// permissionNames names permission p, as driver.Permission numbers
	// it, as this Android names it: reading music is its own
	// permission since Android 13, and part of reading storage before;
	// 2 records from the microphone.
	static String[] permissionNames(int p) {
		if (p == 1) {
			return Build.VERSION.SDK_INT >= 33
				? new String[] {"android.permission.READ_MEDIA_AUDIO"}
				: new String[] {"android.permission.READ_EXTERNAL_STORAGE"};
		}
		if (p == 2) {
			return new String[] {"android.permission.RECORD_AUDIO"};
		}
		return null;
	}

	static boolean permitted(int p) {
		String[] names = permissionNames(p);
		if (names == null || app == null) {
			return false;
		}
		for (String n : names) {
			if (app.checkSelfPermission(n) != PackageManager.PERMISSION_GRANTED) {
				return false;
			}
		}
		return true;
	}

	// ask asks the user for permission p with the system's prompt; the
	// answer goes to Go as answered(code, granted), at once where there
	// is no activity to ask over.
	static void ask(int p, int code) {
		ui.post(() -> {
			String[] names = permissionNames(p);
			GunimActivity a = activity;
			if (names == null || a == null) {
				answered(code, permitted(p));
				return;
			}
			a.requestPermissions(names, code);
		});
	}

	// chooseFolder shows the system's chooser of folders; the folder
	// chosen goes to Go as chosen(code, path), with null for none.
	static void chooseFolder(int code) {
		ui.post(() -> {
			GunimActivity a = activity;
			if (a == null) {
				chosen(code, null);
				return;
			}
			try {
				a.startActivityForResult(new android.content.Intent(android.content.Intent.ACTION_OPEN_DOCUMENT_TREE), code);
			} catch (android.content.ActivityNotFoundException e) {
				chosen(code, null);
			}
		});
	}

	// creating holds when each of the codes of the pickers of a place for
	// a file was asked for, on the UI thread.
	static final java.util.Map<Integer, Long> creating = new java.util.HashMap<>();

	// strays are those of a process that ended before the answer.
	static final java.util.Map<Integer, Long> strays = new java.util.HashMap<>();

	// createDocument shows the system's picker of a place to save a file
	// in, and saveFolder its chooser of a folder to write in. What the user
	// chose goes to Go as saved(code, place, name, made), with null for
	// nothing.
	static void createDocument(int code, String name, String type) {
		ui.post(() -> {
			android.content.Intent pick = new android.content.Intent(android.content.Intent.ACTION_CREATE_DOCUMENT);
			pick.addCategory(android.content.Intent.CATEGORY_OPENABLE);
			pick.setType(type.isEmpty() ? GunimFiles.typeOf(name) : type);
			pick.putExtra(android.content.Intent.EXTRA_TITLE, name);
			pickPlace(code, pick);
		});
	}

	static void saveFolder(int code) {
		ui.post(() -> pickPlace(code, new android.content.Intent(android.content.Intent.ACTION_OPEN_DOCUMENT_TREE)));
	}

	private static void pickPlace(int code, android.content.Intent pick) {
		GunimActivity a = activity;
		if (a == null) {
			saved(code, null, "", false);
			return;
		}
		try {
			creating.put(code, System.currentTimeMillis());
			a.startActivityForResult(pick, code);
		} catch (android.content.ActivityNotFoundException e) {
			creating.remove(code);
			saved(code, null, "", false);
		}
	}

	// picked takes the place the user chose, apart from the UI thread.
	static void picked(int code, long asked, android.net.Uri u) {
		if (u == null) {
			saved(code, null, "", false);
			return;
		}
		new Thread(() -> {
			boolean folder = DocumentsContract.isTreeUri(u);
			String about = folder ? "\0" + 0 : GunimSave.about(u, asked, "");
			boolean made = about.endsWith("1");
			if (folder || made) {
				GunimSave.use(u);
			}
			saved(code, u.toString(), about.substring(0, about.indexOf('\0')), made);
		}).start();
	}

	// save does one of GunimSave's calls for Go, by its number there.
	static String save(int what, String a, String b, String c, String d, String e) {
		switch (what) {
		case 0:
			return GunimSave.open(a);
		case 1:
			return GunimSave.finish(Long.parseLong(a), !b.isEmpty());
		case 2:
			GunimSave.abandon(Long.parseLong(a));
			return "";
		case 3:
			return GunimSave.delete(a) ? "1" : "";
		case 4:
			GunimSave.letGo(a);
			return "";
		case 5:
			return GunimSave.rootOf(a);
		case 6:
			return GunimSave.find(a, b, c);
		case 7:
			return GunimSave.mkdir(a, b, c);
		case 8:
			return GunimSave.create(a, b, c, d, e.isEmpty() ? GunimFiles.typeOf(c) : e);
		case 9:
			return GunimSave.rename(a, b);
		case 10:
			GunimSave.forget(a);
			return "";
		}
		return "";
	}

	// choosingFiles holds the codes of the document pickers showing, on
	// the UI thread, to tell their answers from the folder chooser's, and
	// whether each answers with streams.
	static final java.util.Map<Integer, Boolean> choosingFiles = new java.util.HashMap<>();

	// chooseFiles shows the system's document picker; the files chosen
	// go to Go through GunimOpen. patterns are the kinds of file to show,
	// each ended by a NUL.
	static void chooseFiles(int code, boolean multiple, String patterns, boolean streams) {
		ui.post(() -> {
			GunimActivity a = activity;
			if (a == null) {
				noFiles(code, streams);
				return;
			}
			android.content.Intent pick = new android.content.Intent(android.content.Intent.ACTION_OPEN_DOCUMENT);
			pick.addCategory(android.content.Intent.CATEGORY_OPENABLE);
			pick.setType("*/*");
			String[] types = GunimOpen.typesOf(patterns);
			if (types != null) {
				pick.putExtra(android.content.Intent.EXTRA_MIME_TYPES, types);
			}
			pick.putExtra(android.content.Intent.EXTRA_ALLOW_MULTIPLE, multiple);
			try {
				choosingFiles.put(code, streams);
				a.startActivityForResult(pick, code);
			} catch (android.content.ActivityNotFoundException e) {
				choosingFiles.remove(code);
				noFiles(code, streams);
			}
		});
	}

	// noFiles answers a chooser of files with none.
	static void noFiles(int code, boolean streams) {
		if (streams) {
			streams(code, new int[0], new long[0], new long[0], new long[0], "", "", "");
		} else {
			files(code, new int[0], new long[0], "", "");
		}
	}

	// streamEnd says how the sender of a stream ended, after a wait for
	// its word where asked, and closeStream lets the stream go.
	static int streamEnd(long id, boolean wait) {
		return GunimStreams.end(id, wait);
	}

	static void closeStream(long id) {
		GunimStreams.close(id);
	}

	// pathOf returns the path of a folder the chooser gave, where it
	// lies on the phone's storage or a card: the chooser names those
	// as their volume and the path within it, as primary:Download.
	// Other providers' folders have no path, and give null.
	static String pathOf(android.net.Uri tree) {
		if (tree == null || !"com.android.externalstorage.documents".equals(tree.getAuthority())
			|| !android.provider.DocumentsContract.isTreeUri(tree)) {
			return null;
		}
		String id = android.provider.DocumentsContract.getTreeDocumentId(tree);
		int colon = id.indexOf(':');
		if (colon < 0) {
			return null;
		}
		String volume = id.substring(0, colon), rest = id.substring(colon + 1);
		String root = "primary".equals(volume)
			? Environment.getExternalStorageDirectory().getAbsolutePath()
			: "/storage/" + volume;
		return rest.isEmpty() ? root : root + "/" + rest;
	}

	// userFolder returns the shared folder of kind f, as
	// driver.UserFolder numbers it, or null.
	static String userFolder(int f) {
		if (f == 1) {
			return Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_MUSIC).getAbsolutePath();
		}
		return null;
	}

	// noBackupDir returns the program's folder that Android keeps out of
	// backups, which it makes the first time, or null.
	static String noBackupDir() {
		java.io.File dir = app == null ? null : app.getNoBackupFilesDir();
		return dir == null ? null : dir.getPath();
	}

	// fingers says each finger is a finger of its own, and a second one
	// starts no pinch. Go sets it through takeFingers to what the window the
	// touch started in takes, as each touch starts.
	static volatile boolean fingers;

	static void takeFingers(boolean on) {
		fingers = on;
	}

	// awake says the activity keeps the screen on. It lives here, as a
	// flag on the activity's window goes with the activity, and a new one
	// reads it through applyAwake as it starts.
	static volatile boolean awake;

	static void keepAwake(boolean on) {
		awake = on;
		ui.post(() -> {
			if (activity != null) {
				activity.applyAwake();
			}
		});
	}

	// secure says the activity keeps what it shows to the screen. It lives
	// here for a new activity, as awake does.
	static volatile boolean secure;

	// secure sets that, and returns once the screen shows the activity so:
	// 0, or 1 where other applications' windows may stay over it, or 2
	// where that took too long to know. On the UI thread it does not wait.
	static int secure(boolean on) {
		java.util.concurrent.CountDownLatch done = new java.util.concurrent.CountDownLatch(1);
		int[] over = new int[1];
		Runnable apply = () -> {
			secure = on;
			GunimActivity a = activity;
			if (a == null) {
				done.countDown();
				return;
			}
			a.applySecure((alone) -> {
				over[0] = alone ? 0 : 1;
				done.countDown();
			});
		};
		if (Looper.myLooper() == Looper.getMainLooper()) {
			apply.run();
			return over[0];
		}
		ui.post(apply);
		try {
			return done.await(5, java.util.concurrent.TimeUnit.SECONDS) ? over[0] : 2;
		} catch (InterruptedException e) {
			return 2;
		}
	}

	static float[] readLook() {
		return GunimLook.read();
	}

	static void watchLook() {
		GunimLook.watch();
	}

	// backLeft says the next Back is the system's, and leaves the program.
	// It is kept on the UI thread, where a new activity reads it.
	static boolean backLeft;

	static void backLeaves(boolean on) {
		ui.post(() -> {
			backLeft = on;
			if (activity != null) {
				activity.applyBack();
			}
		});
	}

	// leaveApp leaves the program, as the system's Back does from its
	// first activity: the program goes to the background, and runs on.
	static void leaveApp() {
		ui.post(() -> {
			if (activity != null) {
				activity.moveTaskToBack(true);
			}
		});
	}

	static void buzz() {
		ui.post(() -> {
			if (view != null) {
				view.performHapticFeedback(android.view.HapticFeedbackConstants.LONG_PRESS);
			}
		});
	}

	// share shows the system's share sheet with text, a subject and
	// files, the paths each ended by a NUL, and files served, the name and
	// the type of each ended by a NUL. It returns false where there is no
	// activity to show the sheet over.
	static boolean share(String text, String subject, String paths, long[] ids, long[] sizes, String served) {
		GunimActivity a = activity;
		if (a == null) {
			return false;
		}
		java.util.ArrayList<android.net.Uri> uris = new java.util.ArrayList<>();
		String type = null;
		for (String p : paths.split("\0")) {
			if (p.isEmpty()) {
				continue;
			}
			uris.add(GunimFiles.uriOf(a, p));
			type = GunimFiles.commonType(type, GunimFiles.typeOf(p));
		}
		String[] words = served.split("\0", -1);
		for (int i = 0; i < ids.length; i++) {
			android.net.Uri u = GunimServed.uriOf(a, ids[i], words[2 * i], words[2 * i + 1], sizes[i]);
			uris.add(u);
			type = GunimFiles.commonType(type, GunimServed.typeOf(u));
		}
		android.content.Intent send;
		if (uris.size() > 1) {
			send = new android.content.Intent(android.content.Intent.ACTION_SEND_MULTIPLE);
			send.putParcelableArrayListExtra(android.content.Intent.EXTRA_STREAM, uris);
		} else {
			send = new android.content.Intent(android.content.Intent.ACTION_SEND);
			if (uris.size() == 1) {
				send.putExtra(android.content.Intent.EXTRA_STREAM, uris.get(0));
			}
		}
		send.setType(type != null ? type : "text/plain");
		if (!text.isEmpty()) {
			send.putExtra(android.content.Intent.EXTRA_TEXT, text);
		}
		if (!subject.isEmpty()) {
			send.putExtra(android.content.Intent.EXTRA_SUBJECT, subject);
		}
		if (!uris.isEmpty()) {
			// The clip carries the grant to read each file through the
			// chooser to the application the user picks.
			ClipData clip = ClipData.newRawUri(null, uris.get(0));
			for (int i = 1; i < uris.size(); i++) {
				clip.addItem(new ClipData.Item(uris.get(i)));
			}
			send.setClipData(clip);
			send.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION);
		}
		android.content.Intent chooser = android.content.Intent.createChooser(send, subject.isEmpty() ? null : subject);
		ui.post(() -> {
			try {
				a.startActivity(chooser);
			} catch (android.content.ActivityNotFoundException e) {
				// The system has no share sheet; the share goes nowhere.
			}
		});
		return true;
	}

	// openServed hands a file served to another application. It returns 0,
	// 1 where none opens it, and 2 where there is no activity to start one
	// from.
	static int openServed(long id, String name, String type, long size) {
		GunimActivity a = activity;
		if (a == null) {
			return 2;
		}
		android.net.Uri u = GunimServed.uriOf(a, id, name, type, size);
		android.content.Intent view = new android.content.Intent(android.content.Intent.ACTION_VIEW);
		view.setDataAndType(u, GunimServed.typeOf(u));
		view.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION);
		java.util.concurrent.FutureTask<Integer> start = new java.util.concurrent.FutureTask<>(() -> {
			try {
				a.startActivity(view);
				return 0;
			} catch (android.content.ActivityNotFoundException e) {
				return 1;
			}
		});
		if (Looper.myLooper() == Looper.getMainLooper()) {
			start.run();
		} else {
			ui.post(start);
		}
		try {
			return start.get();
		} catch (Exception e) {
			return 1;
		}
	}

	static void stopServed(long id) {
		GunimServed.stop(id);
	}

	// openLink opens the web page at url in the browser the phone keeps
	// for the web. It returns false where there is no activity to start
	// it from. Android 11 and later hide which browsers there are from an
	// app that does not list them, so a phone with none opens nothing.
	static boolean openLink(String url) {
		GunimActivity a = activity;
		if (a == null) {
			return false;
		}
		android.content.Intent view = new android.content.Intent(android.content.Intent.ACTION_VIEW,
			android.net.Uri.parse(url));
		view.addCategory(android.content.Intent.CATEGORY_BROWSABLE);
		ui.post(() -> {
			try {
				a.startActivity(view);
			} catch (android.content.ActivityNotFoundException e) {
				// The browser went away since; the link opens nowhere.
			}
		});
		return true;
	}

	// vibrate runs the vibration motor in a pattern of milliseconds, on
	// and off in turn from on, and an empty one stops it. It returns
	// false where the device has no motor.
	@SuppressWarnings("deprecation")
	static boolean vibrate(long[] pattern) {
		if (app == null) {
			return false;
		}
		android.os.Vibrator v = (android.os.Vibrator) app.getSystemService(Context.VIBRATOR_SERVICE);
		if (v == null || !v.hasVibrator()) {
			return false;
		}
		long on = 0;
		for (int i = 0; i < pattern.length; i += 2) {
			on += pattern[i];
		}
		v.cancel();
		if (on == 0) {
			return true;
		}
		if (pattern.length == 1) {
			v.vibrate(android.os.VibrationEffect.createOneShot(pattern[0], android.os.VibrationEffect.DEFAULT_AMPLITUDE));
			return true;
		}
		// A waveform starts with a wait, which a pattern starts without.
		long[] timings = new long[pattern.length + 1];
		System.arraycopy(pattern, 0, timings, 1, pattern.length);
		v.vibrate(android.os.VibrationEffect.createWaveform(timings, -1));
		return true;
	}

	// hasCompass reports whether the phone has the sensors that tell
	// which way it faces.
	static boolean hasCompass() {
		return app != null && GunimCompass.of(app).has();
	}

	// watchHeading starts or stops those sensors; while they run, Go
	// hears each reading as heading.
	static void watchHeading(boolean on) {
		ui.post(() -> {
			if (app != null) {
				GunimCompass.of(app).watch(on);
			}
		});
	}

	// finish finishes gunim's activities, as the program is done with its
	// windows. One that comes later waits for a main that runs anew.
	static void finish() {
		ui.post(() -> {
			closing = true;
			for (android.app.Activity a : GunimWatch.live()) {
				if (a instanceof GunimActivity) {
					a.finish();
				}
			}
		});
	}

	// ended is told once main has returned.
	static void ended() {
		ui.post(() -> {
			over = true;
			settle();
		});
	}

	// readers is for how many more seconds what comes too late waits for
	// another application to let a file go that this program serves, which
	// it can read no more: Android may end a reader with the process.
	private static int readers = 5;

	// settling says that settle runs again in a while, and starting that
	// an activity which waited is being started again, for the new process
	// to show.
	private static boolean settling, starting;

	// settle ends the process once main has returned and it is of no more
	// use, so that what Android starts next gets a new one, at once. It
	// looks again in a while otherwise.
	private static void settle() {
		if (settling || starting) {
			return;
		}
		if (late ? isSettledLate() : isUnused()) {
			anew();
		} else if (!starting) {
			settling = true;
			ui.postDelayed(() -> {
				settling = false;
				settle();
			}, 1000);
		}
	}

	// isUnused reports whether the process holds nothing of the program's:
	// no activity, gunim's or its own, no service, as a job's, and no file
	// that another application has open. One that has stopped counts too:
	// Android may end the application with the process, though the user
	// looks at it, and nothing needs the process to end.
	@SuppressWarnings("deprecation")
	private static boolean isUnused() {
		if (!GunimWatch.live().isEmpty() || GunimServed.isOpen()) {
			return false;
		}
		android.app.ActivityManager m = (android.app.ActivityManager) app.getSystemService(Context.ACTIVITY_SERVICE);
		// It tells of the program's own services alone.
		for (android.app.ActivityManager.RunningServiceInfo s : m.getRunningServices(Integer.MAX_VALUE)) {
			if (s.pid == android.os.Process.myPid()) {
				return false;
			}
		}
		return true;
	}

	// isSettledLate reports whether the process may end for what came too
	// late in it, which Android starts again: once no activity of the
	// program's own shows, as Android forgets one whose process ends while
	// it does. It forgets gunim's too, so one that waits and shows is
	// started again first, and ends the process as it is created.
	private static boolean isSettledLate() {
		if (GunimServed.isOpen() && readers > 0) {
			// A reader that hears of an error lets its file go.
			readers--;
			GunimServed.end();
			return false;
		}
		GunimActivity waiting = null;
		for (android.app.Activity a : GunimWatch.live()) {
			if (!GunimWatch.isShown(a) || a.isFinishing()) {
				continue;
			}
			if (!(a instanceof GunimActivity)) {
				return false;
			}
			waiting = (GunimActivity) a;
		}
		if (waiting == null) {
			return true;
		}
		starting = true;
		waiting.finish();
		waiting.startActivity(new android.content.Intent(waiting.getIntent()));
		// Where Android refuses the start, as for one just left, the
		// process ends all the same.
		ui.postDelayed(Native::anew, 5000);
		return false;
	}
}
