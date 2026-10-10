import net.minecraft.SharedConstants;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.server.Bootstrap;

/**
 * FnDump — prints the toString of loaded vanilla density functions, so the
 * parameters actually used by the jar can be compared against the datagen JSON
 * LivingWorld embeds.
 *
 * Usage: FnDump <fn ids...>
 */
public class FnDump {
    public static void main(String[] args) {
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();
        var lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        var dfLookup = lookup.lookupOrThrow(Registries.DENSITY_FUNCTION);
        for (String id : args) {
            var key = ResourceKey.create(Registries.DENSITY_FUNCTION, Identifier.parse(id));
            var opt = dfLookup.get(key);
            System.out.println(id + " => " + opt.map(h -> h.value().toString()).orElse("<missing>"));
        }
    }
}
