package gunim.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.drawable.Icon;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.NoSuchFileException;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.util.HashMap;
import java.util.Map;

/**
 * GunimNotify shows the program's messages, and brings the tag of one the
 * user taps back to it.
 *
 * Any application can start the activity, so a tap shows that it is one by
 * a random word, which the program's private storage keeps and only Android
 * holds besides, in the message.
 */
final class GunimNotify {
	// What post answers.
	static final int SHOWN = 0, OFF = 1, FAILED = 2;

	private static final String TAG = "gunim.tag", WORD = "gunim.word";

	// The channel of a message of no kind in particular.
	private static final String CHANNEL = "gunim.messages";

	// ID goes with each message's tag.
	private static final int ID = 1;

	private static final int WORD_BYTES = 16, MAX_CODES = 256;

	private static final SecureRandom random = new SecureRandom();

	// codes tell the tags' intents apart to Android, which looks at no
	// extra. They hold for this run: a message that shows keeps its own.
	private static final Map<String, Integer> codes = new HashMap<>();

	private static String word;

	private GunimNotify() {
	}

	// manager returns the manager of notifications, or null with no
	// application yet.
	private static NotificationManager manager() {
		return Native.app == null ? null : Native.app.getSystemService(NotificationManager.class);
	}

	// isOn reports whether the user has the program's messages on.
	static boolean isOn() {
		NotificationManager m = manager();
		return m != null && m.areNotificationsEnabled();
	}

	// isOn reports whether the user has the program's messages on, and
	// those of the channel id, where there is one.
	static boolean isOn(String id) {
		NotificationManager m = manager();
		if (m == null || !isOn()) {
			return false;
		}
		NotificationChannel c = m.getNotificationChannel(id.isEmpty() ? CHANNEL : id);
		return c == null || c.getImportance() != NotificationManager.IMPORTANCE_NONE;
	}

	/**
	 * post shows a message. s holds the tag, the channel's ID, name and
	 * description, the title, the body, and the title and the body for the
	 * lock screen. parts is how far work has come in a thousand, -1 for
	 * work of unknown length, and -2 for none. icon is a PNG, or null.
	 */
	static int post(String[] s, int importance, int lock, int parts, boolean ongoing, byte[] icon) {
		Context app = Native.app;
		NotificationManager m = manager();
		if (m == null) {
			return FAILED;
		}
		try {
			String id = s[1].isEmpty() ? CHANNEL : s[1];
			channel(app, m, id, s[2], s[3], importance);
			if (!isOn(id)) {
				return OFF;
			}
			Icon small = small(icon);
			Notification.Builder b = new Notification.Builder(app, id)
				.setSmallIcon(small)
				.setContentTitle(s[4])
				.setContentText(s[5])
				.setContentIntent(tap(app, s[0]))
				.setOnlyAlertOnce(true)
				.setAutoCancel(!ongoing)
				.setOngoing(ongoing);
			if (parts > -2) {
				b.setProgress(1000, Math.max(parts, 0), parts < 0);
				b.setCategory(Notification.CATEGORY_PROGRESS);
			} else {
				b.setStyle(new Notification.BigTextStyle().bigText(s[5]));
			}
			switch (lock) {
			case 1:
				b.setVisibility(Notification.VISIBILITY_PUBLIC);
				break;
			case 2:
				b.setVisibility(Notification.VISIBILITY_SECRET);
				break;
			default:
				b.setVisibility(Notification.VISIBILITY_PRIVATE);
				b.setPublicVersion(new Notification.Builder(app, id)
					.setSmallIcon(small)
					.setContentTitle(s[6].isEmpty() ? label(app) : s[6])
					.setContentText(s[7])
					.build());
			}
			m.notify(s[0], ID, b.build());
			return SHOWN;
		} catch (RuntimeException e) {
			return FAILED;
		}
	}

	/** end takes the message with tag away. */
	static void end(String tag) {
		NotificationManager m = manager();
		if (m != null) {
			m.cancel(tag, ID);
		}
	}

	private static void channel(Context app, NotificationManager m, String id, String name, String text, int importance) {
		int level = importance == 1 ? NotificationManager.IMPORTANCE_LOW
			: importance == 2 ? NotificationManager.IMPORTANCE_HIGH : NotificationManager.IMPORTANCE_DEFAULT;
		NotificationChannel c = new NotificationChannel(id, name.isEmpty() ? label(app) : name, level);
		c.setDescription(text);
		// Android keeps what the user chose for one it has.
		m.createNotificationChannel(c);
	}

	private static CharSequence label(Context app) {
		return app.getApplicationInfo().loadLabel(app.getPackageManager());
	}

	private static Icon small(byte[] png) {
		Bitmap b = png == null ? null : BitmapFactory.decodeByteArray(png, 0, png.length);
		return b != null ? Icon.createWithBitmap(b)
			: Icon.createWithResource("android", android.R.drawable.ic_popup_reminder);
	}

	/**
	 * tap returns the intent that opens the program and brings tag to it. It
	 * throws an IllegalStateException where the word cannot be kept, as with
	 * a full disk: no tap would be known for one.
	 */
	static PendingIntent tap(Context c, String tag) {
		String with;
		try {
			with = word(c, true);
		} catch (IOException e) {
			throw new IllegalStateException("the word of a tap cannot be kept", e);
		}
		return PendingIntent.getActivity(c, code(tag), open(c)
			.putExtra(TAG, tag)
			.putExtra(WORD, with),
			PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
	}

	// open returns the intent the launcher starts the program with, which
	// goes to the activity there is, with what is over it closed.
	private static Intent open(Context c) {
		return new Intent(Intent.ACTION_MAIN)
			.addCategory(Intent.CATEGORY_LAUNCHER)
			.setClassName(c.getPackageName(), GunimActivity.class.getName())
			.setFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
	}

	private static synchronized int code(String tag) {
		Integer code = codes.get(tag);
		if (code == null) {
			if (codes.size() >= MAX_CODES) {
				codes.clear();
			}
			do {
				code = random.nextInt();
				// 0 is the media notification's.
			} while (code == 0 || codes.containsValue(code));
			codes.put(tag, code);
		}
		return code;
	}

	/**
	 * tapped tells Go of the tap that i is, where it is one. Android starts
	 * a task from Recents with the intent that first started it, whose tap
	 * the program has heard.
	 */
	static void tapped(Context c, Intent i) {
		if (i == null || (i.getFlags() & Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY) != 0) {
			return;
		}
		String tag, with;
		try {
			tag = i.getStringExtra(TAG);
			with = i.getStringExtra(WORD);
		} catch (RuntimeException | OutOfMemoryError | StackOverflowError e) {
			// Any application can send what cannot be read.
			return;
		}
		if (tag == null || with == null) {
			return;
		}
		String own;
		try {
			own = word(c, false);
		} catch (IOException e) {
			// What it was for is the program's, and goes into no log.
			android.util.Log.w("gunim", "a tap on a notification could not be known for one");
			return;
		}
		if (own != null && MessageDigest.isEqual(own.getBytes(StandardCharsets.UTF_8), with.getBytes(StandardCharsets.UTF_8))) {
			Native.tapped(tag);
		}
	}

	// word returns the word that shows a tap to be one, made the first time
	// where asked, or null where there is none. One that cannot be read is
	// not made anew, as messages that show hold it.
	private static synchronized String word(Context c, boolean make) throws IOException {
		if (word != null) {
			return word;
		}
		File f = new File(c.getNoBackupFilesDir(), "gunim-notify");
		try {
			byte[] b = Files.readAllBytes(f.toPath());
			if (b.length == 2 * WORD_BYTES) {
				word = new String(b, StandardCharsets.UTF_8);
				return word;
			}
			// It holds no word, and no message does.
		} catch (NoSuchFileException e) {
			// There is none yet.
		}
		if (!make) {
			return null;
		}
		byte[] b = new byte[WORD_BYTES];
		random.nextBytes(b);
		StringBuilder hex = new StringBuilder();
		for (byte x : b) {
			hex.append(String.format(java.util.Locale.ROOT, "%02x", x & 0xff));
		}
		File part = new File(f.getPath() + ".part");
		try (FileOutputStream out = new FileOutputStream(part)) {
			out.write(hex.toString().getBytes(StandardCharsets.UTF_8));
			out.getFD().sync();
		}
		if (!part.renameTo(f)) {
			throw new IOException("the word was not kept");
		}
		word = hex.toString();
		return word;
	}
}
