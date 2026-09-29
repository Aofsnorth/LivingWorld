import net.minecraft.SharedConstants;
import net.minecraft.core.HolderLookup;
import net.minecraft.core.LayeredRegistryAccess;
import net.minecraft.core.Registry;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.resources.RegistryDataLoader;
import net.minecraft.server.Bootstrap;
import net.minecraft.server.RegistryLayer;
import net.minecraft.server.packs.PackType;
import net.minecraft.server.packs.repository.ServerPacksSource;
import net.minecraft.server.packs.resources.MultiPackResourceManager;
import net.minecraft.tags.TagLoader;
import net.minecraft.tags.TagNetworkSerialization;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;

/**
 * SyncDataDump — replicates the vanilla 26.3 server's registry+tag boot
 * (vanilla pack over the classpath) and dumps the two payloads a server sends
 * during the configuration phase:
 *
 *  1. The synchronized registries with their element ids in raw-registry-id
 *     order (RegistryDataLoader.SYNCHRONIZED_REGISTRIES over the loaded WORLD
 *     layer). Sending the entries in this order makes the client-side raw ids
 *     identical to a vanilla server's.
 *  2. The complete network tag payload (TagNetworkSerialization
 *     .serializeTagsToNetwork) with members as raw registry ids.
 *
 * Output lines (TAB separated):
 *   E\t<registry>\t<id>;<id>;...
 *   G\t<registry>\t<tag>\t<id>,<id>,...
 *
 * Usage: SyncDataDump <out.txt>
 */
public class SyncDataDump {
    public static void main(String[] args) throws Exception {
        if (args.length < 1) {
            System.out.println("usage: SyncDataDump <out.txt>");
            System.exit(2);
        }
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        // Vanilla pack resource manager over the classpath data/ directory —
        // exactly what ServerPacksSource.createVanillaPackSource() serves.
        var rm = new MultiPackResourceManager(PackType.SERVER_DATA,
                List.of(ServerPacksSource.createVanillaPackSource().fullResources()));

        // Server boot: static layer (live BuiltInRegistries), tags for its
        // existing registries, then the WORLD layer loaded from the pack.
        LayeredRegistryAccess<RegistryLayer> layers = RegistryLayer.createRegistryAccess();
        var staticAccess = layers.getLayer(RegistryLayer.STATIC);
        var pendingStaticTags = TagLoader.loadTagsForExistingRegistries(rm, staticAccess);
        // Bind tags into the live static registries so they serialize later.
        for (var pending : pendingStaticTags) {
            pending.apply();
        }
        var loadingLookups = TagLoader.buildUpdatedLookups(
                layers.getAccessForLoading(RegistryLayer.WORLD), pendingStaticTags);
        var worldAccess = RegistryDataLoader.load(
                rm, loadingLookups, RegistryDataLoader.WORLD_REGISTRIES, Runnable::run).join();
        layers = layers.replaceFrom(RegistryLayer.WORLD, worldAccess);

        List<String> lines = new ArrayList<>();

        // 1. Synchronized registries, elements in raw-id order.
        for (RegistryDataLoader.RegistryData<?> data : RegistryDataLoader.SYNCHRONIZED_REGISTRIES) {
            ResourceKey<? extends Registry<?>> key = data.key();
            String regId = key.identifier().toString();
            var opt = layers.compositeAccess().lookup(key);
            if (opt.isEmpty()) {
                System.out.println("MISSING SYNC REGISTRY: " + regId);
                System.exit(1);
            }
            List<String> ids = new ArrayList<>();
            Registry<Object> reg = (Registry<Object>) opt.get();
            for (int i = 0; i < reg.size(); i++) {
                var v = reg.byId(i);
                var k = reg.getKey(v);
                if (k == null) {
                    System.out.println("UNMAPPED ID " + i + " IN " + regId);
                    System.exit(1);
                }
                ids.add(k.toString());
            }
            lines.add("E\t" + regId + "\t" + String.join(";", ids));
            System.out.println("E " + regId + " -> " + ids.size() + " elements");
        }

        // 2. Complete network tag payload, members as raw ids.
        var tags = TagNetworkSerialization.serializeTagsToNetwork(layers);
        for (var e : tags.entrySet()) {
            String regId = e.getKey().identifier().toString();
            var payload = e.getValue();
            var tagMap = payload.tags();
            List<String> names = new ArrayList<>(tagMap.keySet().size());
            for (var tagId : tagMap.keySet()) {
                names.add(tagId.toString());
            }
            names.sort(String::compareTo);
            for (String name : names) {
                var intList = tagMap.get(Identifier.parse(name));
                List<String> members = new ArrayList<>(intList.size());
                intList.forEach(id -> members.add(Integer.toString(id)));
                lines.add("G\t" + regId + "\t" + name + "\t" + String.join(",", members));
            }
            System.out.println("G " + regId + " -> " + names.size() + " tags");
        }

        Files.write(Path.of(args[0]), String.join("\n", lines).getBytes(StandardCharsets.UTF_8));
        System.out.println("wrote " + lines.size() + " lines to " + args[0]);
    }
}
