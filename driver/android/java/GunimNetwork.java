package gunim.android;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.LinkAddress;
import android.net.LinkProperties;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.NetworkInfo;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import java.net.InetAddress;
import java.util.Collections;
import java.util.HashSet;
import java.util.Set;

/**
 * GunimNetwork tells Go of the network the program's connections use, and
 * of whether Android lets the program use it. All but watch runs on the UI
 * thread.
 */
@SuppressWarnings("deprecation")
final class GunimNetwork extends ConnectivityManager.NetworkCallback {
	// watching is what listens, or null.
	private static volatile GunimNetwork watching;

	private static final Handler ui = new Handler(Looper.getMainLooper());

	private final ConnectivityManager cm;

	// run is the number Go gave the listening. What Go hears of one that it
	// stopped, from a call under way, it leaves out by this.
	private final long run;

	// current is the network, or null, of which caps, link and blocked
	// are. Android tells of all three as it names one, and settled says that
	// it has. heard says that it has told of anything.
	private Network current;
	private NetworkCapabilities caps;
	private String link = "";
	private Set<InetAddress> addresses = Collections.emptySet();
	private boolean blocked, settled, heard;

	// waits is how many more times none waits for Android's word.
	private int waits = 10;

	// told is the network, its interface and its addresses Go heard of last.
	private Network told;
	private String toldLink = "";
	private Set<InetAddress> toldAddresses = Collections.emptySet();

	private GunimNetwork(ConnectivityManager cm, long run) {
		this.cm = cm;
		this.run = run;
	}

	/**
	 * watch starts or stops listening, as the run numbered, and returns
	 * false where Android refuses. Go calls it for one at a time.
	 */
	static boolean watch(long run, boolean on) {
		if (Native.app == null) {
			return false;
		}
		ConnectivityManager cm = Native.app.getSystemService(ConnectivityManager.class);
		if (!on) {
			GunimNetwork was = watching;
			watching = null;
			if (was != null) {
				cm.unregisterNetworkCallback(was);
			}
			return true;
		}
		GunimNetwork n = new GunimNetwork(cm, run);
		watching = n;
		try {
			cm.registerDefaultNetworkCallback(n, ui);
		} catch (RuntimeException e) {
			// The manifest lacks network-state, or too many listen.
			watching = null;
			return false;
		}
		ui.postDelayed(n::none, 300);
		return true;
	}

	// none says that there is no network, where Android says nothing as
	// listening starts, as it does with none.
	private void none() {
		if (watching != this || heard) {
			return;
		}
		if (info() != null && waits-- > 0) {
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
		addresses = Collections.emptySet();
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
		// A name server that is another leaves what is connected be, and so
		// does an address more.
		link = String.valueOf(p.getInterfaceName());
		addresses = new HashSet<>();
		for (LinkAddress a : p.getLinkAddresses()) {
			addresses.add(a.getAddress());
		}
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
			addresses = Collections.emptySet();
			blocked = false;
			tell();
		}
	}

	private void tell() {
		if (watching != this) {
			return;
		}
		if (current == null) {
			told = null;
			Native.network(run, 0, false, false, false, false, false);
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
		boolean moved = current.equals(told) && (!link.equals(toldLink) || !addresses.containsAll(toldAddresses));
		told = current;
		toldAddresses = addresses;
		toldLink = link;
		Native.network(run, current.getNetworkHandle(), true, caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED),
			!free, off, moved);
	}
}
