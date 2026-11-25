package org.gunim.background;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.job.JobInfo;
import android.app.job.JobParameters;
import android.app.job.JobScheduler;
import android.app.job.JobService;
import android.content.ComponentName;
import android.content.Context;
import android.net.Network;
import android.os.Handler;
import android.os.Looper;

/**
 * Work is the job the program works in: one the user starts, which
 * Android lets go on with the program out of sight, and starts again
 * after it ends the program's process.
 */
public final class Work extends JobService {
	private static final int ID = 1;
	private static final String CHANNEL = "work";
	private static final Handler ui = new Handler(Looper.getMainLooper());

	// now is the job running and params its parameters, on the UI thread.
	private static Work now;
	private JobParameters params;

	// What Go hears: work may go on, over network, and must stop.
	private static native void started(long network);
	private static native void stopped();

	/** schedule asks Android to start the job, and returns whether it will. */
	static boolean schedule(Context c) {
		JobInfo job = new JobInfo.Builder(ID, new ComponentName(c, Work.class))
			.setUserInitiated(true)
			.setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
			.setEstimatedNetworkBytes(0, 1 << 20)
			.setPersisted(true)
			.build();
		return c.getSystemService(JobScheduler.class).schedule(job) == JobScheduler.RESULT_SUCCESS;
	}

	/** finished tells Android the work is done. Go calls it from a thread of its own. */
	static void finished() {
		ui.post(() -> {
			if (now != null) {
				now.jobFinished(now.params, false);
				now = null;
			}
		});
	}

	@Override
	public boolean onStartJob(JobParameters p) {
		// Android may have started the process for this job alone.
		gunim.android.Native.boot(this);
		now = this;
		params = p;
		NotificationManager m = getSystemService(NotificationManager.class);
		m.createNotificationChannel(new NotificationChannel(CHANNEL, "Work", NotificationManager.IMPORTANCE_LOW));
		Notification n = new Notification.Builder(this, CHANNEL)
			.setSmallIcon(android.R.drawable.stat_sys_upload)
			.setContentTitle("Working")
			.build();
		setNotification(p, ID, n, JOB_END_NOTIFICATION_POLICY_REMOVE);
		started(handle(p));
		return true;
	}

	@Override
	public void onNetworkChanged(JobParameters p) {
		params = p;
		started(handle(p));
	}

	@Override
	public boolean onStopJob(JobParameters p) {
		now = null;
		stopped();
		return true;
	}

	private static long handle(JobParameters p) {
		Network n = p.getNetwork();
		return n == null ? 0 : n.getNetworkHandle();
	}
}
