import net.minecraft.SharedConstants;
import net.minecraft.resources.RegistryDataLoader;
import net.minecraft.server.Bootstrap;

/**
 * Codecs — prints which codec RegistryData.elementCodec() actually returns for
 * every synchronized registry, compared by reference identity against the known
 * static codec fields. The client decodes network registry entries with
 * elementCodec(), so any validation must use the SAME codec.
 */
public class Codecs {
    public static void main(String[] args) {
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        Object[] known = {
                net.minecraft.world.level.dimension.DimensionType.NETWORK_CODEC,
                net.minecraft.world.level.dimension.DimensionType.DIRECT_CODEC,
                net.minecraft.world.level.dimension.DimensionType.CODEC,
                net.minecraft.world.timeline.Timeline.NETWORK_CODEC,
                net.minecraft.world.timeline.Timeline.DIRECT_CODEC,
                net.minecraft.world.timeline.Timeline.CODEC,
                net.minecraft.world.clock.WorldClock.DIRECT_CODEC,
                net.minecraft.world.clock.WorldClock.CODEC,
                net.minecraft.world.item.equipment.trim.TrimMaterial.DIRECT_CODEC,
                net.minecraft.world.item.equipment.trim.TrimMaterial.CODEC,
        };
        String[] knownNames = {
                "DimensionType.NETWORK_CODEC", "DimensionType.DIRECT_CODEC", "DimensionType.CODEC",
                "Timeline.NETWORK_CODEC", "Timeline.DIRECT_CODEC", "Timeline.CODEC",
                "WorldClock.DIRECT_CODEC", "WorldClock.CODEC",
                "TrimMaterial.DIRECT_CODEC", "TrimMaterial.CODEC",
        };

        for (RegistryDataLoader.RegistryData<?> d : RegistryDataLoader.SYNCHRONIZED_REGISTRIES) {
            Object ec = d.elementCodec();
            StringBuilder sb = new StringBuilder();
            sb.append(d.key()).append(" -> ").append(ec.getClass().getName());
            for (int i = 0; i < known.length; i++) {
                if (known[i] == ec) sb.append("  == ").append(knownNames[i]);
            }
            System.out.println(sb);
        }
    }
}
