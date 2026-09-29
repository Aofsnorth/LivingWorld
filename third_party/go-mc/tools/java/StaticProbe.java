import net.minecraft.SharedConstants;
import net.minecraft.core.RegistryAccess;
import net.minecraft.core.registries.BuiltInRegistries;
import net.minecraft.server.Bootstrap;

/**
 * Probe: after Bootstrap.bootStrap(), do the static data-driven registries
 * (damage_type, block_transformer, timeline, ...) already contain their
 * vanilla-pack contents? If yes, the boot path loads the classpath vanilla
 * pack into BuiltInRegistries, which lets a harness build a fully tagged,
 * fully populated layered access without running a real server.
 */
public class StaticProbe {
    public static void main(String[] args) {
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();
        var access = RegistryAccess.fromRegistryOfRegistries(BuiltInRegistries.REGISTRY);
        access.registries().forEach(e -> System.out.println("REG " + e.key().identifier() + " -> " + e.value().size()));
    }
}
