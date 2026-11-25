package gunim.android;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.NetworkInfo;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;

/**
 * GunimNetwork tells Go of the network the program's connections use, and
 * of whether Android lets the program use it. All but watch runs on the UI
 * thread.
 */
@SuppressWarnings("deprecation")
final class GunimNetwork extends ConnectivityManager.NetworkCallback {
	// watching is what listens, or null.
	private static GunimNetwork watching;

	private static final Handler ui = new Handler(Looper.getMainLooper());

	private final ConnectivityManager cm;
	private boolean stopped;

	// current is the network, or null, of which caps, link and blocked
	// are. Android tells of all three as it names one, and settled says that
	// it has. heard says that it has told of anything.
	private Network current;
	private NetworkCapabilities caps;
	private String link = "";
	private boolean blocked, settled, heard;

	// told is the network and the link Go heard of last.
	private Network told;
	private String toldLink = "";

	private GunimNetwork(ConnectivityManager cm) {
		this.cm = cm;
	}

	/**
	 * watch starts or stops listening, and returns false where Android
	 * refuses. Go calls it for one at a time.
	 */
	static boolean watch(boolean on) {
		if (Native.app == null) {
			return false;
		}
		ConnectivityManager cm = Native.app.getSystemService(ConnectivityManager.class);
		if (!on) {
			GunimNetwork was = watching;
			watching = null;
			if (was != null) {
				cm.unregisterNetworkCallback(was);
				ui.post(() -> was.stopped = true);
			}
			return true;
		}
		GunimNetwork n = new GunimNetwork(cm);
		try {
			cm.registerDefaultNetworkCallback(n, ui);
		} catch (RuntimeException e) {
			// The manifest lacks network-state, or too many listen.
			return false;
		}
		watching = n;
		ui.postDelayed(n::none, 300);
		return true;
	}

	// none says that there is no network, where Android says nothing as
	// listening starts, as it does with none.
	private void none() {
		if (stopped || heard) {
			return;
		}
		if (info() != null) {
			// There is one, and it is yet to be told of.
			ui.postDelayed(this::none, 300);
			return;
		}
		heard = true;
		tell();
	}

	private NetworkInfo info() {
		try {
			return cm.getActiveNetworkInfo();
		} catch (RuntimeException e) {
			return null;
		}
	}

	@Override
	public void onAvailable(Network n) {
		current = n;
		caps = null;
		link = "";
		blocked = false;
		settled = false;
		heard = true;
	}

	@Override
	public void onCapabilitiesChanged(Network n, NetworkCapabilities c) {
		if (n.equals(current)) {
			caps = c;
			tell();
		}
	}

	@Override
	public void onLinkPropertiesChanged(Network n, LinkProperties p) {
		if (!n.equals(current)) {
			return;
		}
		link = p.getInterfaceName() + p.getLinkAddresses() + p.getDnsServers();
		if (Build.VERSION.SDK_INT < 29) {
			settled = true;
		}
		tell();
	}

	@Override
	public void onBlockedStatusChanged(Network n, boolean b) {
		if (n.equals(current)) {
			blocked = b;
			settled = true;
			tell();
		}
	}

	@Override
	public void onLost(Network n) {
		if (n.equals(current)) {
			current = null;
			caps = null;
			link = "";
			blocked = false;
			tell();
		}
	}

	private void tell() {
		if (stopped) {
			return;
		}
		if (current == null) {
			told = null;
			Native.network(0, false, false, false, false, false);
			return;
		}
		if (!settled || caps == null) {
			return;
		}
		boolean off = blocked;
		if (Build.VERSION.SDK_INT < 29) {
			NetworkInfo i = info();
			off = i != null && i.getDetailedState() == NetworkInfo.DetailedState.BLOCKED;
		}
		boolean free = caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)
			|| Build.VERSION.SDK_INT >= 30 && caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_TEMPORARILY_NOT_METERED);
		boolean moved = current.equals(told) && !link.equals(toldLink);
		told = current;
		toldLink = link;
		Native.network(current.getNetworkHandle(), true, caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED),
			!free, off, moved);
	}
}
