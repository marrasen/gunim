package gunim.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.os.IBinder;

/**
 * GunimService keeps a gunim program running while its activity is in
 * the background, as a music player playing: a foreground service, with
 * a notification that says so and brings the program back when tapped.
 * Go starts it, changes it and stops it through Native.keepRunning.
 */
public class GunimService extends Service {
	private static final String CHANNEL = "gunim.running";
	private static final int ID = 1;

	// running is the service while it runs, for update to reach.
	private static GunimService running;

	@Override
	public IBinder onBind(Intent i) {
		return null;
	}

	@Override
	public int onStartCommand(Intent i, int flags, int startId) {
		String title = i != null ? i.getStringExtra("title") : null;
		String text = i != null ? i.getStringExtra("text") : null;
		Notification n = notification(title, text);
		if (android.os.Build.VERSION.SDK_INT >= 29) {
			startForeground(ID, n, android.content.pm.ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK);
		} else {
			startForeground(ID, n);
		}
		running = this;
		return START_NOT_STICKY;
	}

	@Override
	public void onDestroy() {
		if (running == this) {
			running = null;
		}
		super.onDestroy();
	}

	/**
	 * update changes the notification of the service running, and
	 * reports whether one was: a service already in the foreground needs
	 * no start, which Android would refuse from the background.
	 */
	static boolean update(String title, String text) {
		GunimService s = running;
		if (s == null) {
			return false;
		}
		NotificationManager nm = s.getSystemService(NotificationManager.class);
		nm.notify(ID, s.notification(title, text));
		return true;
	}

	private Notification notification(String title, String text) {
		NotificationManager nm = getSystemService(NotificationManager.class);
		if (nm.getNotificationChannel(CHANNEL) == null) {
			CharSequence name = getApplicationInfo().loadLabel(getPackageManager());
			NotificationChannel c = new NotificationChannel(CHANNEL, name, NotificationManager.IMPORTANCE_LOW);
			c.setShowBadge(false);
			nm.createNotificationChannel(c);
		}
		Intent open = getPackageManager().getLaunchIntentForPackage(getPackageName());
		PendingIntent tap = PendingIntent.getActivity(this, 0, open,
			PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
		return new Notification.Builder(this, CHANNEL)
			.setSmallIcon(android.R.drawable.ic_media_play)
			.setContentTitle(title != null ? title : "")
			.setContentText(text != null ? text : "")
			.setContentIntent(tap)
			.setOngoing(true)
			.setCategory(Notification.CATEGORY_TRANSPORT)
			.build();
	}
}
