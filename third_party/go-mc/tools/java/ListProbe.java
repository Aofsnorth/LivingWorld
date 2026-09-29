import net.minecraft.SharedConstants;
import net.minecraft.resources.RegistryDataLoader;
import net.minecraft.server.Bootstrap;

/**
 * Probe: list the registry keys in each RegistryDataLoader registry set
 * (WORLD / DIMENSION / SYNCHRONIZED / RELOADABLE) to understand which layer
 * owns which synced registry on a real vanilla server.
 */
public class ListProbe {
    public static void main(String[] args) {
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();
        dump("WORLD", RegistryDataLoader.WORLD_REGISTRIES);
        dump("DIMENSION", RegistryDataLoader.DIMENSION_REGISTRIES);
        dump("SYNCHRONIZED", RegistryDataLoader.SYNCHRONIZED_REGISTRIES);
        dump("RELOADABLE", RegistryDataLoader.RELOADABLE_REGISTRIES);
    }

    private static void dump(String name, java.util.List<RegistryDataLoader.RegistryData<?>> list) {
        StringBuilder sb = new StringBuilder(name + " (" + list.size() + "): ");
        for (RegistryDataLoader.RegistryData<?> d : list) {
            sb.append(d.key().identifier()).append(' ');
        }
        System.out.println(sb);
    }
}
