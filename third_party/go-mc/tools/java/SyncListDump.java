import net.minecraft.SharedConstants;
import net.minecraft.core.Registry;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.ResourceKey;
import net.minecraft.resources.RegistryDataLoader;
import net.minecraft.server.Bootstrap;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;

/**
 * SyncListDump — dumps the exact set of registries a vanilla 26.3 server
 * synchronizes (RegistryDataLoader.SYNCHRONIZED_REGISTRIES) together with the
 * ordered list of element ids in each registry, as loaded from the vanilla
 * pack via VanillaRegistries.createWorldLookup(). This is the authoritative
 * list a vanilla server sends (as data-less PackedRegistryEntry when the
 * client agreed to the same known pack).
 *
 * Output format (one line per registry):
 *   <registry id>\t<element id>;<element id>;...
 *
 * Usage: SyncListDump <out.txt>
 */
public class SyncListDump {
    public static void main(String[] args) throws Exception {
        if (args.length < 1) {
            System.out.println("usage: SyncListDump <out.txt>");
            System.exit(2);
        }
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        var lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();

        List<String> lines = new ArrayList<>();
        for (RegistryDataLoader.RegistryData<?> data : RegistryDataLoader.SYNCHRONIZED_REGISTRIES) {
            ResourceKey<? extends Registry<?>> key = data.key();
            String regId = key.identifier().toString();
            List<String> ids = new ArrayList<>();
            lookup.lookup(key).ifPresent(reg -> reg.listElementIds().forEach(id ->
                    ids.add(id.identifier().toString())));
            lines.add(regId + "\t" + String.join(";", ids));
            System.out.println(regId + " -> " + ids.size() + " elements");
        }
        Files.write(Path.of(args[0]), String.join("\n", lines).getBytes(java.nio.charset.StandardCharsets.UTF_8));
        System.out.println("wrote " + lines.size() + " registries to " + args[0]);
    }
}
